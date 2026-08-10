package agentproc

import (
	"time"

	"github.com/gollem-dev/agentkit"
)

// ClaimAtForTest exposes the claim-time derivation so its branches can be
// pinned without going through Firestore. It is the single reason the claim
// query needs no composite index, so each branch is worth asserting directly.
var ClaimAtForTest = claimAtFor

// ClaimNeverForTest is the sentinel a non-claimable row stores.
var ClaimNeverForTest = claimNever

// ClaimImmediatelyForTest is the sentinel a row runnable right now stores.
var ClaimImmediatelyForTest = claimImmediately

// HashIDForTest exposes the document-id derivation.
var HashIDForTest = hashID

// NewProcessRowForTest builds the stored shape so a test can assert what the
// repository would write for a given Process.
func NewProcessRowForTest(p *agentkit.Process) (agentkit.Process, time.Time) {
	row := processRow{Process: *p, ClaimAt: claimAtFor(p)}
	return row.Process, row.ClaimAt
}
