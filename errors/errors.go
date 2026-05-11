package errors

import "fmt"

// APIError — Generic API error
type APIError struct {
	Code    string
	Message string
	Err     error
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewAPIError(code, message string) *APIError {
	return &APIError{Code: code, Message: message}
}

func WrapAPIError(code, message string, err error) *APIError {
	return &APIError{Code: code, Message: message, Err: err}
}

// StoreError — Storage layer errors
type StoreError struct {
	Code    string
	Message string
	Err     error
}

func (e *StoreError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("store %s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("store %s: %s", e.Code, e.Message)
}

func NewStoreError(code, message string) *StoreError {
	return &StoreError{Code: code, Message: message}
}

func WrapStoreError(code, message string, err error) *StoreError {
	return &StoreError{Code: code, Message: message, Err: err}
}

// ConversationError — Conversation store errors
type ConversationError struct {
	Code    string
	Message string
	Err     error
}

func (e *ConversationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("conversation %s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("conversation %s: %s", e.Code, e.Message)
}

func NewConversationError(code, message string) *ConversationError {
	return &ConversationError{Code: code, Message: message}
}

func WrapConversationError(code, message string, err error) *ConversationError {
	return &ConversationError{Code: code, Message: message, Err: err}
}

var (
	ErrSQLiteError = func(format string, args ...interface{}) *ConversationError {
		return NewConversationError("SQLITE_ERROR", fmt.Sprintf(format, args...))
	}
	ErrLLMError = func(format string, args ...interface{}) *ConversationError {
		return NewConversationError("LLM_ERROR", fmt.Sprintf(format, args...))
	}
)

// AgentError — Agent collection errors
type AgentError struct {
	Code    string
	Message string
	Err     error
}

func (e *AgentError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("agent %s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("agent %s: %s", e.Code, e.Message)
}

func NewAgentError(code, message string) *AgentError {
	return &AgentError{Code: code, Message: message}
}

// GraphError — LangGraph demo errors
type GraphError struct {
	Code    string
	Message string
	Err     error
}

func (e *GraphError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("graph %s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("graph %s: %s", e.Code, e.Message)
}

func NewGraphError(code, message string) *GraphError {
	return &GraphError{Code: code, Message: message}
}

// BM25Error — BM25 store errors
type BM25Error struct {
	Code    string
	Message string
	Err     error
}

func (e *BM25Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("bm25 %s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("bm25 %s: %s", e.Code, e.Message)
}

func NewBM25Error(code, message string) *BM25Error {
	return &BM25Error{Code: code, Message: message}
}

// HybridError — Hybrid store errors
type HybridError struct {
	Code    string
	Message string
	Err     error
}

func (e *HybridError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("hybrid %s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("hybrid %s: %s", e.Code, e.Message)
}

func NewHybridError(code, message string) *HybridError {
	return &HybridError{Code: code, Message: message}
}

// TestError — Test errors
type TestError struct {
	Message string
	Err     error
}

func (e *TestError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("test error: %s (%v)", e.Message, e.Err)
	}
	return fmt.Sprintf("test error: %s", e.Message)
}
