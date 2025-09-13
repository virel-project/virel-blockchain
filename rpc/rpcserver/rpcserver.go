package rpcserver

import (
	"net/http"

	"github.com/virel-project/virel-blockchain/v3/util/ratelimit"
)

type Server struct {
	handlers map[string]Handler
	config   Config

	limit *ratelimit.Limit
}
type Handler = func(c *Context)

type Config struct {
	// When true, the RPC server will block CORS requests and require valid username and password.
	Restricted bool

	// The username:password used in Basic Auth. Leave blank to disable authentication.
	Authentication string

	// The maximum number of requests per minute from a single IP address. Default is 500.
	RateLimit int
}

func New(bind string, config Config) *Server {
	if config.RateLimit == 0 {
		config.RateLimit = 500
	}

	rpcSrv := &Server{
		handlers: make(map[string]func(c *Context)),
		config:   config,
		limit:    ratelimit.New(config.RateLimit),
	}

	httpSrv := &http.Server{
		Addr: bind,
	}
	go httpSrv.ListenAndServe()

	httpSrv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rpcSrv.handler(w, r)
	})

	return rpcSrv
}

func (s *Server) Handle(method string, f Handler) {
	s.handlers[method] = f
}
