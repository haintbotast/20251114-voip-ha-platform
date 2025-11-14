package cdr

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
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

// InsertCDR nhận raw JSON từ FreeSWITCH và insert vào bảng voip.cdr, voip.recordings.
func InsertCDR(ctx context.Context, pool *pgxpool.Pool, raw []byte) error {
    var fs FreeSwitchCDR
    if err := json.Unmarshal(raw, &fs); err != nil {
        return err
    }

    layout := "2006-01-02 15:04:05"
    start, _ := time.ParseInLocation(layout, fs.Variables.StartStamp, time.Local)
    end, _ := time.ParseInLocation(layout, fs.Variables.EndStamp, time.Local)
    answer, _ := time.ParseInLocation(layout, fs.Variables.AnswerStamp, time.Local)

    dur := atoiSafe(fs.Variables.Duration)
    bill := atoiSafe(fs.Variables.BillSec)

    var recordingID *int64
    if fs.Variables.RecordingFile != "" {
        var id int64
        err := pool.QueryRow(ctx, `
            INSERT INTO voip.recordings (call_uuid, path, backend)
            VALUES ($1, $2, 'local')
            ON CONFLICT (call_uuid, path) DO UPDATE
            SET path = EXCLUDED.path
            RETURNING id
        `, fs.Variables.UUID, fs.Variables.RecordingFile).Scan(&id)
        if err == nil {
            recordingID = &id
        }
    }

    _, err := pool.Exec(ctx, `
        INSERT INTO voip.cdr (
            call_uuid, direction,
            caller_id_number, destination_number,
            start_time, answer_time, end_time,
            duration, billsec, hangup_cause, recording_id, raw_json
        ) VALUES (
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12
        )
        ON CONFLICT (call_uuid) DO NOTHING
    `,
        fs.Variables.UUID,
        fs.Variables.Direction,
        fs.Variables.CallerIDNumber,
        fs.Variables.DestinationNumber,
        start,
        answer,
        end,
        dur,
        bill,
        fs.Variables.HangupCause,
        recordingID,
        raw,
    )
    return err
}

func atoiSafe(s string) int {
    var n int
    _, _ = fmt.Sscanf(s, "%d", &n)
    return n
}
