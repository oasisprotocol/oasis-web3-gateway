package txpool

// API is the txpool_ prefixed set of APIs in the Web3 JSON-RPC spec.
type API interface {
	// Content returns the (always empty) contents of the txpool.
	Content() (map[string][]interface{}, error)
}

type publicAPI struct{}

// NewPublicAPI creates an instance of the Web3 API.
func NewPublicAPI() API {
	return &publicAPI{}
}

func (api *publicAPI) Content() (map[string][]interface{}, error) {
	m := make(map[string][]interface{})
	m["pending"] = []interface{}{}
	return m, nil
}
