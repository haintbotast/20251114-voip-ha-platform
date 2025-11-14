package cdr

import (
	"context"
	"errors"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v4"
)

func TestInsertCDR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       []byte
		setupMock func(pgxmock.PgxPoolIface)
		wantErr   error
	}{
		{
			name: "happy path with queue agent and recording",
			raw: []byte(`{
                "variables": {
                    "uuid": "uuid-1",
                    "direction": "inbound",
                    "caller_id_number": "1001",
                    "destination_number": "2002",
                    "start_stamp": "2024-05-01 10:00:00",
                    "answer_stamp": "2024-05-01 10:00:05",
                    "end_stamp": "2024-05-01 10:01:00",
                    "duration": "60",
                    "billsec": "55",
                    "hangup_cause": "NORMAL_CLEARING",
                    "queue_name": "Support",
                    "agent_id": "AG01",
                    "recording_file": "/tmp/rec.wav"
                }
            }`),
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT id FROM voip\.queues WHERE name = \$1`).
					WithArgs("Support").
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(10)))
				mock.ExpectQuery(`SELECT id FROM voip\.agent_users WHERE external_id = \$1`).
					WithArgs("AG01").
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(20)))
				mock.ExpectQuery(`INSERT INTO voip\.recordings`).
					WithArgs("uuid-1", "/tmp/rec.wav").
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(30)))
				mock.ExpectExec(`INSERT INTO voip\.cdr`).
					WithArgs(
						"uuid-1",
						"inbound",
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						60,
						55,
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
					).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				mock.ExpectCommit()
			},
			wantErr: nil,
		},
		{
			name: "invalid timestamp",
			raw: []byte(`{
                "variables": {
                    "uuid": "uuid-2",
                    "direction": "outbound",
                    "start_stamp": "bad-time",
                    "end_stamp": "2024-05-01 10:01:00"
                }
            }`),
			setupMock: nil,
			wantErr:   ErrInvalidCDRData,
		},
		{
			name: "duplicate uuid",
			raw: []byte(`{
                "variables": {
                    "uuid": "uuid-3",
                    "direction": "outbound",
                    "start_stamp": "2024-05-01 10:00:00",
                    "end_stamp": "2024-05-01 10:00:30",
                    "duration": "30",
                    "billsec": "25"
                }
            }`),
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()
				mock.ExpectExec(`INSERT INTO voip\.cdr`).
					WithArgs(
						"uuid-3",
						"outbound",
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						30,
						25,
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
					).
					WillReturnResult(pgxmock.NewResult("INSERT", 0))
				mock.ExpectRollback()
			},
			wantErr: ErrDuplicateCDR,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("failed to create pgx mock: %v", err)
			}
			defer mock.Close()

			if tc.setupMock != nil {
				tc.setupMock(mock)
			}

			err = InsertCDR(context.Background(), mock, tc.raw)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet expectations: %v", err)
			}
		})
	}
}
