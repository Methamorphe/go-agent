package api

import (
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	"github.com/Methamorphe/go-agent/internal/process"
)

const (
	TypeProcessCreate  = "process.create"
	TypeProcessInspect = "process.inspect"
	TypeProcessSuspend = "process.suspend"
	TypeProcessResume  = "process.resume"
	TypeProcessEvents  = "process.events"
)

/*
Alias conservés pour le client CLI.

À terme, on pourra choisir une seule convention de nommage.
Pour l'instant :

	TypeProcessCreate
	MessageProcessCreate

représentent exactement la même chaîne :

	"process.create"
*/
const (
	MessageProcessCreate  = TypeProcessCreate
	MessageProcessInspect = TypeProcessInspect
	MessageProcessSuspend = TypeProcessSuspend
	MessageProcessResume  = TypeProcessResume
	MessageProcessEvents  = TypeProcessEvents
)

/*
ProcessCreateRequest est envoyé par :

	go-agent process create --intent "..."

À ce stade de G1, un processus root est créé avec un Intent
humain durable.
*/
type ProcessCreateRequest struct {
	Intent string `json:"intent"`
}

/*
ProcessIDRequest est utilisé pour les commandes qui ont seulement
besoin d'identifier un Agent Process.

Exemple :

	process inspect <agent-id>
*/
type ProcessIDRequest struct {
	AgentID id.AgentID `json:"agent_id"`
}

/*
ProcessTransitionRequest est utilisé pour suspend/resume.

ExpectedVersion est un pointeur volontairement.

nil signifie :

	aucune version attendue explicitement fournie

Alors que :

	&version

signifie que le client demande un Compare-And-Swap sur cette
version précise.
*/
type ProcessTransitionRequest struct {
	AgentID id.AgentID `json:"agent_id"`

	ExpectedVersion *uint64 `json:"expected_version,omitempty"`

	Reason string `json:"reason,omitempty"`
}

/*
ProcessEventsRequest permet de lire le ledger d'un processus.

AfterVersion est exclusif.

Donc :

	AfterVersion = 3

retourne les événements :

	4, 5, 6, ...
*/
type ProcessEventsRequest struct {
	AgentID id.AgentID `json:"agent_id"`

	AfterVersion uint64 `json:"after_version,omitempty"`

	Limit int `json:"limit,omitempty"`
}

/*
ProcessResponse est la réponse commune des opérations qui
retournent l'état canonique courant.
*/
type ProcessResponse struct {
	Process process.State `json:"process"`
}

/*
ProcessEventsResponse retourne les événements durables du ledger
dans l'ordre de process_version.
*/
type ProcessEventsResponse struct {
	Events []ledger.Event `json:"events"`
}
