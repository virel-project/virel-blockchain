package walletrpc

func (r *RpcClient) GetBalance(p GetBalanceRequest) (*GetBalanceResponse, error) {
	o := &GetBalanceResponse{}

	return o, r.Request("get_balance", p, o)
}
