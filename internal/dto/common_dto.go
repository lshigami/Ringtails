package dto

type ErrorResponse struct {
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
