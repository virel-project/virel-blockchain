package walletrpc

func (r *RpcClient) GetBalance(p GetBalanceRequest) (*GetBalanceResponse, error) {
	o := &GetBalanceResponse{}
	return o, r.Request("get_balance", p, o)
}

func (r *RpcClient) GetHistory(p GetHistoryRequest) (*GetHistoryResponse, error) {
	o := &GetHistoryResponse{}
	return o, r.Request("get_history", p, o)
}

func (r *RpcClient) CreateTransaction(p CreateTransactionRequest) (*CreateTransactionResponse, error) {
	o := &CreateTransactionResponse{}
	return o, r.Request("create_transaction", p, o)
}

func (r *RpcClient) SubmitTransaction(p SubmitTransactionRequest) (*SubmitTransactionResponse, error) {
	o := &SubmitTransactionResponse{}
	return o, r.Request("submit_transaction", p, o)
}

func (r *RpcClient) GetSubaddress(p GetSubaddressRequest) (*GetSubaddressResponse, error) {
	o := &GetSubaddressResponse{}
	return o, r.Request("get_subaddress", p, o)
}
