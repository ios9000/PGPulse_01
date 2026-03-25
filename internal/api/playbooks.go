package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/auth"
	"github.com/ios9000/PGPulse_01/internal/playbook"
)

// handleListPlaybooks returns all playbooks with optional filters.
func (s *APIServer) handleListPlaybooks(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	opts := playbook.PlaybookListOpts{
		Status:   r.URL.Query().Get("status"),
		Category: r.URL.Query().Get("category"),
		Search:   r.URL.Query().Get("search"),
		Limit:    queryInt(r, "limit", 50),
		Offset:   queryInt(r, "offset", 0),
	}
	playbooks, total, err := s.playbookStore.List(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LIST_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"playbooks": playbooks,
		"total":     total,
	})
}

// handleGetPlaybook returns a single playbook with all steps.
func (s *APIServer) handleGetPlaybook(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid playbook ID")
		return
	}
	pb, err := s.playbookStore.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "GET_ERROR", err.Error())
		return
	}
	if pb == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "playbook not found")
		return
	}
	writeJSON(w, http.StatusOK, pb)
}

// handleCreatePlaybook creates a new playbook (starts as draft).
func (s *APIServer) handleCreatePlaybook(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	var pb playbook.Playbook
	if err := json.NewDecoder(r.Body).Decode(&pb); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	pb.Status = "draft"
	pb.Version = 1
	claims := auth.ClaimsFromContext(r.Context())
	if claims != nil {
		pb.Author = claims.Username
	}
	id, err := s.playbookStore.Create(r.Context(), &pb)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CREATE_ERROR", err.Error())
		return
	}
	pb.ID = id
	writeJSON(w, http.StatusCreated, pb)
}

// handleUpdatePlaybook updates a playbook (bumps version, resets to draft).
func (s *APIServer) handleUpdatePlaybook(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid playbook ID")
		return
	}
	var pb playbook.Playbook
	if err := json.NewDecoder(r.Body).Decode(&pb); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	pb.ID = id
	claims := auth.ClaimsFromContext(r.Context())
	if claims != nil {
		pb.Author = claims.Username
	}
	if err := s.playbookStore.Update(r.Context(), &pb); err != nil {
		writeError(w, http.StatusInternalServerError, "UPDATE_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleDeletePlaybook deletes a playbook (blocked for builtins).
func (s *APIServer) handleDeletePlaybook(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid playbook ID")
		return
	}
	// Check if builtin.
	pb, err := s.playbookStore.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "GET_ERROR", err.Error())
		return
	}
	if pb == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "playbook not found")
		return
	}
	if pb.IsBuiltin {
		writeError(w, http.StatusForbidden, "BUILTIN", "cannot delete built-in playbooks")
		return
	}
	if err := s.playbookStore.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "DELETE_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handlePromotePlaybook promotes a draft playbook to stable.
func (s *APIServer) handlePromotePlaybook(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid playbook ID")
		return
	}
	if err := s.playbookStore.Promote(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "PROMOTE_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "promoted"})
}

// handleDeprecatePlaybook marks a playbook as deprecated.
func (s *APIServer) handleDeprecatePlaybook(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid playbook ID")
		return
	}
	if err := s.playbookStore.Deprecate(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "DEPRECATE_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deprecated"})
}

// handleResolvePlaybook finds the best playbook for a given context.
func (s *APIServer) handleResolvePlaybook(w http.ResponseWriter, r *http.Request) {
	if s.playbookResolver == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	rc := playbook.ResolverContext{
		HookID:       r.URL.Query().Get("hook"),
		RootCauseKey: r.URL.Query().Get("root_cause"),
		MetricKey:    r.URL.Query().Get("metric"),
		AdviserRule:  r.URL.Query().Get("adviser_rule"),
		InstanceID:   r.URL.Query().Get("instance_id"),
	}
	pb, reason, err := s.playbookResolver.Resolve(r.Context(), rc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RESOLVE_ERROR", err.Error())
		return
	}
	resp := map[string]any{"playbook": pb}
	if pb != nil {
		resp["match_reason"] = reason
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleStartRun starts a new playbook run on an instance.
func (s *APIServer) handleStartRun(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil || s.playbookExecutor == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "INSTANCE_NOT_FOUND", "instance not found")
		return
	}

	playbookID, err := strconv.ParseInt(chi.URLParam(r, "playbookId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid playbook ID")
		return
	}

	pb, err := s.playbookStore.Get(r.Context(), playbookID)
	if err != nil || pb == nil {
		writeError(w, http.StatusNotFound, "PLAYBOOK_NOT_FOUND", "playbook not found")
		return
	}

	var body struct {
		TriggerSource string `json:"trigger_source"`
		TriggerID     string `json:"trigger_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	claims := auth.ClaimsFromContext(r.Context())
	username := "anonymous"
	if claims != nil {
		username = claims.Username
	}

	run := &playbook.Run{
		PlaybookID:       pb.ID,
		PlaybookVersion:  pb.Version,
		InstanceID:       instanceID,
		StartedBy:        username,
		Status:           "in_progress",
		CurrentStepOrder: 1,
		TriggerSource:    body.TriggerSource,
		TriggerID:        body.TriggerID,
	}

	runID, err := s.playbookStore.CreateRun(r.Context(), run)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CREATE_RUN_ERROR", err.Error())
		return
	}

	// Pre-create pending step records for all steps.
	for _, step := range pb.Steps {
		rs := &playbook.RunStep{
			RunID:     runID,
			StepOrder: step.StepOrder,
			Status:    "pending",
		}
		if _, err := s.playbookStore.CreateRunStep(r.Context(), rs); err != nil {
			s.logger.Warn("failed to create run step", "run_id", runID, "step", step.StepOrder, "error", err)
		}
	}

	run.ID = runID
	writeJSON(w, http.StatusCreated, run)
}

// handleGetRun returns a run with all step results.
func (s *APIServer) handleGetRun(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	runID, err := strconv.ParseInt(chi.URLParam(r, "runId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid run ID")
		return
	}
	run, err := s.playbookStore.GetRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "GET_RUN_ERROR", err.Error())
		return
	}
	if run == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "run not found")
		return
	}

	// Include playbook steps definition for the wizard.
	pb, err := s.playbookStore.Get(r.Context(), run.PlaybookID)
	if err == nil && pb != nil {
		resp := map[string]any{
			"run":      run,
			"playbook": pb,
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}

// handleExecuteStep runs a single step.
func (s *APIServer) handleExecuteStep(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil || s.playbookExecutor == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}

	runID, err := strconv.ParseInt(chi.URLParam(r, "runId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid run ID")
		return
	}
	stepOrder, err := strconv.Atoi(chi.URLParam(r, "stepOrder"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_STEP", "invalid step order")
		return
	}

	run, err := s.playbookStore.GetRun(r.Context(), runID)
	if err != nil || run == nil {
		writeError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
		return
	}

	pb, err := s.playbookStore.Get(r.Context(), run.PlaybookID)
	if err != nil || pb == nil {
		writeError(w, http.StatusNotFound, "PLAYBOOK_NOT_FOUND", "playbook not found")
		return
	}

	// Find the step definition.
	var step *playbook.Step
	for i := range pb.Steps {
		if pb.Steps[i].StepOrder == stepOrder {
			step = &pb.Steps[i]
			break
		}
	}
	if step == nil {
		writeError(w, http.StatusNotFound, "STEP_NOT_FOUND", "step not found")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())

	// Tier-based permission check.
	switch step.SafetyTier {
	case playbook.TierDiagnostic:
		// Auto-execute, no extra permission.
	case playbook.TierRemediate:
		var body struct {
			Confirmed bool `json:"confirmed"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Confirmed {
			writeJSON(w, http.StatusOK, map[string]any{
				"status":      "awaiting_confirmation",
				"sql":         step.SQLTemplate,
				"safety_tier": step.SafetyTier,
			})
			return
		}
	case playbook.TierDangerous:
		if claims == nil || !auth.HasPermission(auth.Role(claims.Role), auth.PermInstanceManagement) {
			writeJSON(w, http.StatusOK, map[string]any{
				"status":  "awaiting_approval",
				"message": "This action requires DBA approval",
			})
			return
		}
	case playbook.TierExternal:
		writeJSON(w, http.StatusOK, map[string]any{
			"status":       "manual_action",
			"instructions": step.ManualInstructions,
			"escalation":   step.EscalationContact,
		})
		// Mark step as completed (manual acknowledgment).
		now := time.Now()
		rs := &playbook.RunStep{
			RunID:     runID,
			StepOrder: stepOrder,
			Status:    "completed",
			ExecutedAt: &now,
		}
		_ = s.playbookStore.UpdateRunStep(r.Context(), rs)
		return
	}

	// C5: Concurrency guard — lock step before execution.
	locked, err := s.playbookStore.LockStepForExecution(r.Context(), runID, stepOrder)
	if err != nil || !locked {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "Step is already being executed or has completed",
		})
		return
	}

	// Execute the step.
	result, err := s.playbookExecutor.ExecuteStep(r.Context(), run.InstanceID, *step)
	if err != nil {
		// C6: Error state machine — mark step failed, don't advance.
		now := time.Now()
		rs := &playbook.RunStep{
			RunID:      runID,
			StepOrder:  stepOrder,
			Status:     "failed",
			Error:      err.Error(),
			SQLExecuted: step.SQLTemplate,
			ExecutedAt: &now,
		}
		_ = s.playbookStore.UpdateRunStep(r.Context(), rs)

		writeJSON(w, http.StatusOK, map[string]any{
			"step_result": rs,
			"next_step":   stepOrder,
			"run_status":  "in_progress",
			"can_retry":   true,
		})
		return
	}

	// Interpret results.
	verdict, message := playbook.Interpret(step.ResultInterpretation, result.Columns, result.Rows, result.RowCount)

	// Determine next step.
	nextStep := playbook.EvaluateBranch(*step, verdict, result.Columns, result.Rows)

	// Marshal result JSON.
	resultJSON, _ := json.Marshal(result)

	// Save step result.
	now := time.Now()
	username := "anonymous"
	if claims != nil {
		username = claims.Username
	}
	rs := &playbook.RunStep{
		RunID:         runID,
		StepOrder:     stepOrder,
		Status:        "completed",
		SQLExecuted:   step.SQLTemplate,
		ResultJSON:    resultJSON,
		ResultVerdict: verdict,
		ResultMessage: message,
		ExecutedAt:    &now,
		DurationMs:    result.Duration,
		ConfirmedBy:   username,
	}
	_ = s.playbookStore.UpdateRunStep(r.Context(), rs)

	// Update run state.
	run.CurrentStepOrder = nextStep
	if nextStep == 0 {
		run.Status = "completed"
		completedAt := time.Now()
		run.CompletedAt = &completedAt
	}
	_ = s.playbookStore.UpdateRun(r.Context(), run)

	writeJSON(w, http.StatusOK, map[string]any{
		"step_result": rs,
		"next_step":   nextStep,
		"run_status":  run.Status,
	})
}

// handleConfirmStep confirms a Tier 2 step (instance_management required).
func (s *APIServer) handleConfirmStep(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

// handleApproveStep approves a Tier 3 step (instance_management required).
func (s *APIServer) handleApproveStep(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	runID, _ := strconv.ParseInt(chi.URLParam(r, "runId"), 10, 64)
	stepOrder, _ := strconv.Atoi(chi.URLParam(r, "stepOrder"))

	// Reset step to pending so it can be executed.
	_ = s.playbookStore.ResetStepForRetry(r.Context(), runID, stepOrder)
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// handleRequestApproval sets a step to pending_approval (C7).
func (s *APIServer) handleRequestApproval(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	runID, _ := strconv.ParseInt(chi.URLParam(r, "runId"), 10, 64)
	stepOrder, _ := strconv.Atoi(chi.URLParam(r, "stepOrder"))

	if err := s.playbookStore.RequestStepApproval(r.Context(), runID, stepOrder); err != nil {
		writeError(w, http.StatusInternalServerError, "APPROVAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "pending_approval",
		"message": "Approval requested. A DBA can approve this step from the run URL.",
	})
}

// handleRetryStep resets a failed step for retry (C6).
func (s *APIServer) handleRetryStep(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	runID, _ := strconv.ParseInt(chi.URLParam(r, "runId"), 10, 64)
	stepOrder, _ := strconv.Atoi(chi.URLParam(r, "stepOrder"))

	if err := s.playbookStore.ResetStepForRetry(r.Context(), runID, stepOrder); err != nil {
		writeError(w, http.StatusInternalServerError, "RETRY_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "pending"})
}

// handleSkipStep skips a step.
func (s *APIServer) handleSkipStep(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	runID, _ := strconv.ParseInt(chi.URLParam(r, "runId"), 10, 64)
	stepOrder, _ := strconv.Atoi(chi.URLParam(r, "stepOrder"))

	now := time.Now()
	rs := &playbook.RunStep{
		RunID:      runID,
		StepOrder:  stepOrder,
		Status:     "skipped",
		ExecutedAt: &now,
	}
	_ = s.playbookStore.UpdateRunStep(r.Context(), rs)
	writeJSON(w, http.StatusOK, map[string]string{"status": "skipped"})
}

// handleAbandonRun abandons a run.
func (s *APIServer) handleAbandonRun(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	runID, _ := strconv.ParseInt(chi.URLParam(r, "runId"), 10, 64)

	run, err := s.playbookStore.GetRun(r.Context(), runID)
	if err != nil || run == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "run not found")
		return
	}
	completedAt := time.Now()
	run.Status = "abandoned"
	run.CompletedAt = &completedAt
	_ = s.playbookStore.UpdateRun(r.Context(), run)

	// Implicit feedback: abandoned.
	resolved := false
	_ = s.playbookStore.UpdateFeedback(r.Context(), runID, nil, &resolved, "run abandoned")

	writeJSON(w, http.StatusOK, map[string]string{"status": "abandoned"})
}

// handleSubmitFeedback records explicit user feedback for a run.
func (s *APIServer) handleSubmitFeedback(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	runID, _ := strconv.ParseInt(chi.URLParam(r, "runId"), 10, 64)

	var body struct {
		Useful   *bool  `json:"useful"`
		Resolved *bool  `json:"resolved"`
		Notes    string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	if err := s.playbookStore.UpdateFeedback(r.Context(), runID, body.Useful, body.Resolved, body.Notes); err != nil {
		writeError(w, http.StatusInternalServerError, "FEEDBACK_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "feedback_recorded"})
}

// handleListInstanceRuns lists runs for a specific instance.
func (s *APIServer) handleListInstanceRuns(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	instanceID := chi.URLParam(r, "id")
	opts := playbook.RunListOpts{
		Status: r.URL.Query().Get("status"),
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}
	runs, total, err := s.playbookStore.ListRunsByInstance(r.Context(), instanceID, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LIST_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs, "total": total})
}

// handleListAllRuns lists all runs fleet-wide.
func (s *APIServer) handleListAllRuns(w http.ResponseWriter, r *http.Request) {
	if s.playbookStore == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAYBOOKS_DISABLED", "playbooks not available")
		return
	}
	opts := playbook.RunListOpts{
		Status: r.URL.Query().Get("status"),
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}
	runs, total, err := s.playbookStore.ListRuns(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LIST_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs, "total": total})
}

// queryInt extracts an integer query parameter with a default.
func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
