package server

// Card is a TCG card in API responses.
type Card struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Number    string `json:"number"`
	SetName   string `json:"setName"`
	SetTotal  int    `json:"setTotal"`
	ImageURL  string `json:"imageUrl"`
	DexNumber int    `json:"dexNumber"`
}

// Pokemon is a Pokémon species in API responses.
type Pokemon struct {
	ID         int            `json:"id"`
	Name       string         `json:"name"`
	Types      []string       `json:"types"`
	Stats      map[string]int `json:"stats"`
	FlavorText string         `json:"flavorText"`
	Evolution  Evolution      `json:"evolution"`
}

// Evolution is one node of an evolution chain.
type Evolution struct {
	Name      string      `json:"name"`
	EvolvesTo []Evolution `json:"evolvesTo"`
}

// ScanResponse is the body of a successful POST /v1/scan.
type ScanResponse struct {
	Status     string   `json:"status"`
	Card       *Card    `json:"card,omitempty"`
	Pokemon    *Pokemon `json:"pokemon,omitempty"`
	Candidates []Card   `json:"candidates,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// ErrorBody is the body of every error response.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
