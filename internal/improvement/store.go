package improvement

import (
	"context"
	"time"
)

type Store interface {
	CreateVersion(context.Context, ArtifactVersion) error
	Version(context.Context, ArtifactID, uint32) (ArtifactVersion, error)
	Versions(context.Context, ArtifactID) ([]ArtifactVersion, error)
	SetStatus(context.Context, ArtifactID, uint32, ArtifactStatus) error
	PutEvaluation(context.Context, Evaluation) error
	Evaluation(context.Context, EvaluationID) (Evaluation, error)
	Promote(context.Context, PromotionRecord) error
	Promotion(context.Context, ArtifactID, uint32) (PromotionRecord, error)
	ActiveVersion(context.Context, ArtifactID) (uint32, error)
	Rollback(context.Context, ArtifactID, uint32, uint32, time.Time) error
	BindInvocation(context.Context, InvocationManifest) error
	InvocationManifest(context.Context, string) (InvocationManifest, error)
	KillSwitch(context.Context) (KillSwitch, error)
	SetKillSwitch(context.Context, KillSwitch) error
	ReserveCanaryUse(context.Context, ArtifactID, uint32, uint64) error
}
