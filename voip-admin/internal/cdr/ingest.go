package cdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type FreeSwitchCDR struct {
	Variables struct {
		UUID              string `json:"uuid"`
		Direction         string `json:"direction"`
		CallerIDNumber    string `json:"caller_id_number"`
		DestinationNumber string `json:"destination_number"`
		StartStamp        string `json:"start_stamp"`
		AnswerStamp       string `json:"answer_stamp"`
		EndStamp          string `json:"end_stamp"`
		Duration          string `json:"duration"`
		BillSec           string `json:"billsec"`
		HangupCause       string `json:"hangup_cause"`
		QueueName         string `json:"queue_name"`
		AgentID           string `json:"agent_id"`
		RecordingFile     string `json:"recording_file"`
	} `json:"variables"`
}

// txStarter is the minimal interface needed from a pgx pool for InsertCDR.
type txStarter interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

var (
	// ErrInvalidCDRData được trả về khi dữ liệu đầu vào không hợp lệ.
	ErrInvalidCDRData = errors.New("invalid cdr data")
	// ErrDuplicateCDR được trả về khi call_uuid đã tồn tại.
	ErrDuplicateCDR = errors.New("duplicate cdr")
)

const fsTimestampLayout = "2006-01-02 15:04:05"

// InsertCDR nhận raw JSON từ FreeSWITCH và insert vào bảng voip.cdr, voip.recordings.
func InsertCDR(ctx context.Context, pool txStarter, raw []byte) (err error) {
	var fs FreeSwitchCDR
	if err := json.Unmarshal(raw, &fs); err != nil {
		slog.Error("failed to unmarshal freeswitch cdr", "error", err)
		return fmt.Errorf("%w: %v", ErrInvalidCDRData, err)
	}

	start, err := parseRequiredTimestamp(fs.Variables.StartStamp, "start_stamp")
	if err != nil {
		return err
	}

	end, err := parseRequiredTimestamp(fs.Variables.EndStamp, "end_stamp")
	if err != nil {
		return err
	}

	answer, err := parseOptionalTimestamp(fs.Variables.AnswerStamp, "answer_stamp")
	if err != nil {
		return err
	}

	duration, err := parseIntField(fs.Variables.Duration, "duration")
	if err != nil {
		return err
	}

	billsec, err := parseIntField(fs.Variables.BillSec, "billsec")
	if err != nil {
		return err
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				slog.Error("failed to rollback cdr transaction", "error", rbErr)
			}
			return
		}

		if commitErr := tx.Commit(ctx); commitErr != nil {
			err = fmt.Errorf("commit tx: %w", commitErr)
		}
	}()

	var queueID *int64
	if name := strings.TrimSpace(fs.Variables.QueueName); name != "" {
		var id int64
		err = tx.QueryRow(ctx, `SELECT id FROM voip.queues WHERE name = $1`, name).Scan(&id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				slog.Info("queue not found for cdr", "call_uuid", fs.Variables.UUID, "queue_name", name)
				err = nil
			} else {
				return fmt.Errorf("lookup queue: %w", err)
			}
		} else {
			queueID = &id
			slog.Info("mapped cdr queue", "call_uuid", fs.Variables.UUID, "queue_name", name, "queue_id", id)
		}
	}

	var agentUserID *int64
	if agent := strings.TrimSpace(fs.Variables.AgentID); agent != "" {
		var id int64
		err = tx.QueryRow(ctx, `SELECT id FROM voip.agent_users WHERE external_id = $1`, agent).Scan(&id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				slog.Info("agent not found for cdr", "call_uuid", fs.Variables.UUID, "agent_id", agent)
				err = nil
			} else {
				return fmt.Errorf("lookup agent: %w", err)
			}
		} else {
			agentUserID = &id
			slog.Info("mapped cdr agent", "call_uuid", fs.Variables.UUID, "agent_id", agent, "agent_user_id", id)
		}
	}

	var recordingID *int64
	if file := strings.TrimSpace(fs.Variables.RecordingFile); file != "" {
		var id int64
		if err = tx.QueryRow(ctx, `
            INSERT INTO voip.recordings (call_uuid, path, backend)
            VALUES ($1, $2, 'local')
            ON CONFLICT (call_uuid, path) DO UPDATE
            SET path = EXCLUDED.path
            RETURNING id
        `, fs.Variables.UUID, file).Scan(&id); err != nil {
			return fmt.Errorf("upsert recording: %w", err)
		}
		recordingID = &id
		slog.Info("upserted recording", "call_uuid", fs.Variables.UUID, "path", file, "recording_id", id)
	}

	cmdTag, execErr := tx.Exec(ctx, `
        INSERT INTO voip.cdr (
            call_uuid, direction,
            caller_id_number, destination_number,
            start_time, answer_time, end_time,
            duration, billsec, hangup_cause,
            queue_id, agent_user_id, recording_id, raw_json
        ) VALUES (
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
        )
        ON CONFLICT (call_uuid) DO NOTHING
    `,
		fs.Variables.UUID,
		fs.Variables.Direction,
		nullableString(fs.Variables.CallerIDNumber),
		nullableString(fs.Variables.DestinationNumber),
		start,
		answer,
		end,
		duration,
		billsec,
		nullableString(fs.Variables.HangupCause),
		queueID,
		agentUserID,
		recordingID,
		raw,
	)
	if execErr != nil {
		return fmt.Errorf("insert cdr: %w", execErr)
	}

	if cmdTag.RowsAffected() == 0 {
		slog.Info("cdr already exists", "call_uuid", fs.Variables.UUID)
		return ErrDuplicateCDR
	}

	slog.Info("inserted cdr", "call_uuid", fs.Variables.UUID, "direction", fs.Variables.Direction)
	return nil
}

func parseRequiredTimestamp(value, field string) (time.Time, error) {
	t, err := parseTimestamp(value, field, true)
	if err != nil {
		return time.Time{}, err
	}
	return *t, nil
}

func parseOptionalTimestamp(value, field string) (*time.Time, error) {
	return parseTimestamp(value, field, false)
}

func parseTimestamp(value, field string, required bool) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if required {
			slog.Warn("missing timestamp field", "field", field)
			return nil, fmt.Errorf("%w: missing %s", ErrInvalidCDRData, field)
		}
		return nil, nil
	}

	parsed, err := time.ParseInLocation(fsTimestampLayout, trimmed, time.Local)
	if err != nil {
		slog.Warn("invalid timestamp", "field", field, "value", trimmed, "error", err)
		return nil, fmt.Errorf("%w: invalid %s", ErrInvalidCDRData, field)
	}

	utc := parsed.In(time.UTC)
	return &utc, nil
}

func parseIntField(value, field string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}

	n, err := strconv.Atoi(trimmed)
	if err != nil {
		slog.Warn("invalid integer field", "field", field, "value", trimmed, "error", err)
		return 0, fmt.Errorf("%w: invalid %s", ErrInvalidCDRData, field)
	}

	return n, nil
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
