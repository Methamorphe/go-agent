package id

type AgentID string
type EventID string
type InvocationID string
type ActionID string
type TransactionID string
type WorldID string
type CorrelationID string
type RequestID string

func (id AgentID) String() string       { return string(id) }
func (id EventID) String() string       { return string(id) }
func (id InvocationID) String() string  { return string(id) }
func (id ActionID) String() string      { return string(id) }
func (id TransactionID) String() string { return string(id) }
func (id WorldID) String() string       { return string(id) }
func (id CorrelationID) String() string { return string(id) }
func (id RequestID) String() string     { return string(id) }
