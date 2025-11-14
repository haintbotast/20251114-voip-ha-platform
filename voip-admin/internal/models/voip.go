package models

import "time"

type Domain struct {
    ID        int64     `db:"id"`
    Name      string    `db:"name"`
    IsActive  bool      `db:"is_active"`
    CreatedAt time.Time `db:"created_at"`
    UpdatedAt time.Time `db:"updated_at"`
}

type ExtensionType string

const (
    ExtensionTypeUser      ExtensionType = "user"
    ExtensionTypeQueue     ExtensionType = "queue"
    ExtensionTypeIVR       ExtensionType = "ivr"
    ExtensionTypeVoicemail ExtensionType = "voicemail"
    ExtensionTypeService   ExtensionType = "service"
    ExtensionTypeTrunkOut  ExtensionType = "trunk_out"
)

type Extension struct {
    ID         int64         `db:"id"`
    DomainID   int64         `db:"domain_id"`
    Exten      string        `db:"exten"`
    Type       ExtensionType `db:"type"`
    ServiceRef []byte        `db:"service_ref"`
    NeedMedia  bool          `db:"need_media"`
    IsActive   bool          `db:"is_active"`
}

type CDR struct {
    ID                int64      `db:"id" json:"id"`
    CallUUID          string     `db:"call_uuid" json:"call_uuid"`
    Direction         string     `db:"direction" json:"direction"`
    CallerIDNumber    *string    `db:"caller_id_number" json:"caller_id_number,omitempty"`
    DestinationNumber *string    `db:"destination_number" json:"destination_number,omitempty"`
    StartTime         time.Time  `db:"start_time" json:"start_time"`
    AnswerTime        *time.Time `db:"answer_time" json:"answer_time,omitempty"`
    EndTime           time.Time  `db:"end_time" json:"end_time"`
    Duration          int        `db:"duration" json:"duration"`
    BillSec           int        `db:"billsec" json:"billsec"`
    HangupCause       *string    `db:"hangup_cause" json:"hangup_cause,omitempty"`
    QueueID           *int64     `db:"queue_id" json:"queue_id,omitempty"`
    AgentUserID       *int64     `db:"agent_user_id" json:"agent_user_id,omitempty"`
    TrunkID           *int64     `db:"trunk_id" json:"trunk_id,omitempty"`
    RecordingID       *int64     `db:"recording_id" json:"recording_id,omitempty"`
    CreatedAt         time.Time  `db:"created_at" json:"created_at"`
}

type Recording struct {
    ID        int64     `db:"id" json:"id"`
    CallUUID  string    `db:"call_uuid" json:"call_uuid"`
    Path      string    `db:"path" json:"path"`
    Backend   string    `db:"backend" json:"backend"`
    SizeBytes *int64    `db:"size_bytes" json:"size_bytes,omitempty"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
