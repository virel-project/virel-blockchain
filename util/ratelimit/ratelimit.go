package ratelimit

import (
	"sync"
	"time"
)

type info struct {
	Count     int
	LastClear int64
	BanEnds   int64
}

func New(maxPerMinute int) *Limit {
	return &Limit{
		maxPerMinute: maxPerMinute,
		info:         make(map[string]*info),
	}
}

type Limit struct {
	maxPerMinute int
	info         map[string]*info

	sync.RWMutex
}

func (l *Limit) CanAct(ip string, amount int) bool {
	t := time.Now().Unix()

	l.Lock()
	defer l.Unlock()

	inf := l.info[ip]
	if inf == nil {
		inf = &info{}
		l.info[ip] = inf
	}

	if inf.BanEnds > t {
		return false
	}
	if inf.LastClear+60 < t {
		inf.LastClear = t
		inf.Count = 0
		return true
	}

	inf.Count += amount

	if inf.Count > l.maxPerMinute {
		inf.BanEnds = t + 120
		return false
	}
	return true
}
