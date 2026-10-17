package route

import (
	"context"
	"errors"
	"strconv"
	"time"
)

const acceptanceInventoryBudget = 15 * time.Second
const acceptanceInventoryScript = `$ErrorActionPreference='Stop'; $r=@(Find-NetRoute -RemoteIPAddress '1.1.1.1'); if($r.Count -ne 2){exit 7}; [string][int]$r[0].InterfaceIndex`

// Only fixed enums, durations and process metadata; never addresses or output.
type RealRouteInventory struct {
	Reason        string `json:"reason"`
	ElapsedMS     int64  `json:"elapsed_ms"`
	BudgetMS      int64  `json:"budget_ms"`
	QueryExitCode *int   `json:"query_exit_code"`
	StderrPresent *bool  `json:"stderr_present"`
}

type acceptanceInventoryQuery func(context.Context, string, string, time.Duration) ([]byte, error)

func acceptanceRouteInventory(ctx context.Context, query acceptanceInventoryQuery) (index int, diagnostic RealRouteInventory, err error) {
	started := time.Now()
	diagnostic = RealRouteInventory{Reason: "unexpected_query_error", BudgetMS: acceptanceInventoryBudget.Milliseconds()}
	defer func() { diagnostic.ElapsedMS = time.Since(started).Milliseconds() }()
	raw, queryErr := query(ctx, "acceptance_route_inventory", acceptanceInventoryScript, acceptanceInventoryBudget)
	if queryErr != nil {
		var known *windowsQueryError
		if errors.As(queryErr, &known) {
			stderr := known.StderrPresent
			diagnostic.StderrPresent = &stderr
			switch known.Stage {
			case "start_failed", "timeout", "canceled", "io_failed", "output_limit", "exit_failed":
				diagnostic.Reason = "query_" + known.Stage
			}
			if known.Stage == "exit_failed" || known.Stage == "output_limit" {
				code := known.ExitCode
				diagnostic.QueryExitCode = &code
			}
			if known.Stage == "exit_failed" && known.ExitCode == 7 {
				diagnostic.Reason = "route_shape_rejected"
			}
		}
		return 0, diagnostic, errAcceptance
	}
	zero := 0
	diagnostic.QueryExitCode = &zero
	index, parseErr := strconv.Atoi(string(raw))
	if parseErr != nil {
		diagnostic.Reason = "index_not_integer"
		return 0, diagnostic, errAcceptance
	}
	if index < 1 {
		diagnostic.Reason = "index_not_positive"
		return 0, diagnostic, errAcceptance
	}
	diagnostic.Reason = "none"
	return index, diagnostic, nil
}
