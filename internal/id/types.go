package id

type AgentID string
type EventID string
type InvocationID string
type ActionID string
type TransactionID string
type WorldID string
type CorrelationID string
type RequestID string

type IntentID string
type SleepID string
type LeaseID string
type RuntimeInstanceID string
type SnapshotID string
type CheckpointID string
type ContextPageID string

func (id AgentID) String() string           { return string(id) }
func (id EventID) String() string           { return string(id) }
func (id InvocationID) String() string      { return string(id) }
func (id ActionID) String() string          { return string(id) }
func (id TransactionID) String() string     { return string(id) }
func (id WorldID) String() string           { return string(id) }
func (id CorrelationID) String() string     { return string(id) }
func (id RequestID) String() string         { return string(id) }
func (id IntentID) String() string          { return string(id) }
func (id SleepID) String() string           { return string(id) }
func (id LeaseID) String() string           { return string(id) }
func (id RuntimeInstanceID) String() string { return string(id) }
func (id SnapshotID) String() string        { return string(id) }
func (id CheckpointID) String() string      { return string(id) }
func (id ContextPageID) String() string     { return string(id) }
