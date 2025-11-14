package httpapi

import (
    "encoding/json"
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "voip-admin/internal/models"
)

type CDRResponse struct {
    Items []models.CDR `json:"items"`
}

func CDRQueryHandler(pool *pgxpool.Pool) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        q := r.URL.Query()

        fromStr := q.Get("from")
        toStr := q.Get("to")
        caller := q.Get("caller")
        callee := q.Get("callee")

        limitStr := q.Get("limit")
        if limitStr == "" {
            limitStr = "100"
        }
        limit, _ := strconv.Atoi(limitStr)
        if limit <= 0 {
            limit = 100
        }
        if limit > 1000 {
            limit = 1000
        }

        var (
            where []string
            args  []interface{}
            idx   = 1
        )

        layout := time.RFC3339
        if fromStr != "" {
            if t, err := time.Parse(layout, fromStr); err == nil {
                where = append(where, "start_time >= $"+strconv.Itoa(idx))
                args = append(args, t)
                idx++
            }
        }
        if toStr != "" {
            if t, err := time.Parse(layout, toStr); err == nil {
                where = append(where, "start_time <= $"+strconv.Itoa(idx))
                args = append(args, t)
                idx++
            }
        }
        if caller != "" {
            where = append(where, "caller_id_number = $"+strconv.Itoa(idx))
            args = append(args, caller)
            idx++
        }
        if callee != "" {
            where = append(where, "destination_number = $"+strconv.Itoa(idx))
            args = append(args, callee)
            idx++
        }

        query := "SELECT id, call_uuid, direction, caller_id_number, destination_number, start_time, answer_time, end_time, duration, billsec, hangup_cause, queue_id, agent_user_id, trunk_id, recording_id, created_at FROM voip.cdr"
        if len(where) > 0 {
            query += " WHERE " + strings.Join(where, " AND ")
        }
        query += " ORDER BY start_time DESC LIMIT " + strconv.Itoa(limit)

        rows, err := pool.Query(r.Context(), query, args...)
        if err != nil {
            http.Error(w, "query error", http.StatusInternalServerError)
            return
        }
        defer rows.Close()

        res := CDRResponse{}
        for rows.Next() {
            var c models.CDR
            if err := rows.Scan(
                &c.ID, &c.CallUUID, &c.Direction,
                &c.CallerIDNumber, &c.DestinationNumber,
                &c.StartTime, &c.AnswerTime, &c.EndTime,
                &c.Duration, &c.BillSec, &c.HangupCause,
                &c.QueueID, &c.AgentUserID, &c.TrunkID, &c.RecordingID,
                &c.CreatedAt,
            ); err != nil {
                http.Error(w, "scan error", http.StatusInternalServerError)
                return
            }
            res.Items = append(res.Items, c)
        }

        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(res)
    }
}
