package daemonrpc

func (r *RpcClient) GetTransaction(p GetTransactionRequest) (*GetTransactionResponse, error) {
	o := &GetTransactionResponse{}

	return o, r.Request("get_transaction", p, o)
}

func (r *RpcClient) GetInfo(p GetInfoRequest) (*GetInfoResponse, error) {
	o := &GetInfoResponse{}
	return o, r.Request("get_info", p, &o)
}

func (r *RpcClient) GetAddress(p GetAddressRequest) (*GetAddressResponse, error) {
	o := &GetAddressResponse{}
	return o, r.Request("get_address", p, &o)
}

func (r *RpcClient) GetTxList(p GetTxListRequest) (*GetTxListResponse, error) {
	o := &GetTxListResponse{}
	return o, r.Request("get_tx_list", p, &o)
}

func (r *RpcClient) SubmitTransaction(p SubmitTransactionRequest) (*SubmitTransactionResponse, error) {
	o := &SubmitTransactionResponse{}
	return o, r.Request("submit_transaction", p, &o)
}

func (r *RpcClient) GetBlockByHash(p GetBlockByHashRequest) (*GetBlockResponse, error) {
	o := &GetBlockResponse{}
	return o, r.Request("get_block_by_hash", p, &o)
}

func (r *RpcClient) GetBlockByHeight(p GetBlockByHeightRequest) (*GetBlockResponse, error) {
	o := &GetBlockResponse{}
	return o, r.Request("get_block_by_height", p, &o)
}

func (r *RpcClient) CalcPow(p CalcPowRequest) (*CalcPowResponse, error) {
	o := &CalcPowResponse{}
	return o, r.Request("calc_pow", p, &o)
}

func (r *RpcClient) ValidateAddress(p ValidateAddressRequest) (*ValidateAddressResponse, error) {
	o := &ValidateAddressResponse{}
	return o, r.Request("validate_address", p, &o)
}

func (r *RpcClient) GetRichList(p RichListRequest) (*RichListResponse, error) {
	o := &RichListResponse{}
	return o, r.Request("get_rich_list", p, &o)
}

func (r *RpcClient) SubmitStakeSignature(p SubmitStakeSignatureRequest) (*SubmitStakeSignatureResponse, error) {
	o := &SubmitStakeSignatureResponse{}
	return o, r.Request("submit_stake_signature", p, &o)
}
