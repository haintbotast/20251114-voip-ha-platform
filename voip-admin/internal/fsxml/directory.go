package fsxml

import (
    "context"
    "errors"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"
)

type DirectoryService struct {
    Pool *pgxpool.Pool
}

// BuildDirectory trả về document XML cho directory của FreeSWITCH.
// Đây là ví dụ đơn giản, mapping user từ DB -> password + caller ID.
func (d *DirectoryService) BuildDirectory(ctx context.Context, user, domain string) (*Document, error) {
    if d.Pool == nil {
        return nil, errors.New("db pool is nil")
    }

    var password, fullName string
    err := d.Pool.QueryRow(ctx, `
        SELECT u.sip_password, COALESCE(u.full_name, u.username) AS full_name
        FROM voip.users u
        JOIN voip.domains d ON d.id = u.domain_id
        WHERE u.username=$1
          AND d.name=$2
          AND u.is_active=TRUE
        LIMIT 1
    `, user, domain).Scan(&password, &fullName)
    if err != nil {
        return nil, err
    }

    doc := &Document{
        Type: "freeswitch/xml",
        Section: []Section{
            {
                Name: "directory",
                Domain: &DomainNode{
                    Name: domain,
                    User: []UserNode{
                        {
                            ID: user,
                            Params: []ParamNode{
                                {Name: "password", Value: password},
                            },
                            Vars: []VariableNode{
                                {Name: "user_context", Value: "default"},
                                {Name: "effective_caller_id_name", Value: fullName},
                                {Name: "effective_caller_id_number", Value: user},
                                {Name: "outbound_caller_id_number", Value: user},
                                {Name: "dialplan", Value: "XML"},
                            },
                        },
                    },
                },
            },
        },
    }

    return doc, nil
}

// DebugString chỉ dùng cho debug/log nếu cần.
func (d *Document) DebugString() string {
    return fmt.Sprintf("Document type=%s sections=%d", d.Type, len(d.Section))
}
