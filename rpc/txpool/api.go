package txpool

// PublicAPI is the txpool_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicAPI struct{}

// NewPublicAPI creates an instance of the Web3 API.
func NewPublicAPI() *PublicAPI {
	return &PublicAPI{}
}

// Content returns the (always empty) contents of the txpool.
func (api *PublicAPI) Content() (map[string][]interface{}, error) {
	m := make(map[string][]interface{})
	m["pending"] = []interface{}{}
	return m, nil
}
