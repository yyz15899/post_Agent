package model

type AgentChatRequest struct {
	Message string `json:"message" binding:"required"`
	// ToolCalls string
}
