package process

import "testing"

func TestOperationEventsAdvanceVersionWithoutChangingRunningState(t *testing.T) {
	state := testReadyState(t)
	state = mustApply(t, state, testEvent(t, state, EventAgentActivated, AgentActivatedPayload{
		LeaseID: "lse_test", RuntimeInstanceID: "run_test",
	}))

	before := state.Version
	state = mustApply(t, state, testEvent(t, state, EventModelInvocationStarted, ModelInvocationStartedPayload{
		InvocationID: "inv_test", Provider: "fake", Model: "model", RequestRef: "sha256:req",
	}))
	state = mustApply(t, state, testEvent(t, state, EventModelInvocationCompleted, ModelInvocationCompletedPayload{
		InvocationID: "inv_test", ResponseRef: "sha256:resp", FinishReason: "stop",
	}))
	state = mustApply(t, state, testEvent(t, state, EventSyscallRequested, SyscallRequestedPayload{
		ActionID: "act_test", Name: "observe", ArgumentsRef: "sha256:args",
	}))
	state = mustApply(t, state, testEvent(t, state, EventSyscallCompleted, SyscallCompletedPayload{
		ActionID: "act_test", Name: "observe", Status: "known_succeeded", ResultRef: "sha256:result",
	}))

	if state.Status != StatusRunning {
		t.Fatalf("status = %s, want RUNNING", state.Status)
	}
	if state.Version != before+4 {
		t.Fatalf("version = %d, want %d", state.Version, before+4)
	}
}

func TestOperationEventRequiresRunningProcess(t *testing.T) {
	state := testReadyState(t)
	_, err := Apply(state, testEvent(t, state, EventModelInvocationStarted, ModelInvocationStartedPayload{
		InvocationID: "inv_test", Provider: "fake", Model: "model", RequestRef: "sha256:req",
	}))
	if err == nil {
		t.Fatal("model invocation operation event was accepted while process was not RUNNING")
	}
}
