package fsxml

import (
    "context"
    "errors"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"
)

type DialplanService struct {
    Pool *pgxpool.Pool
}

// BuildDialplan xây dialplan theo destination_number và context.
func (s *DialplanService) BuildDialplan(ctx context.Context, caller, callee, contextName string) (*Document, error) {
    if s.Pool == nil {
        return nil, errors.New("db pool is nil")
    }

    if contextName == "" {
        contextName = "default"
    }

    var extType, serviceRef string
    err := s.Pool.QueryRow(ctx, `
        SELECT type::text, COALESCE(service_ref::text, '')
        FROM voip.extensions e
        JOIN voip.domains d ON d.id=e.domain_id
        WHERE e.exten=$1
          AND e.is_active=TRUE
        LIMIT 1
    `, callee).Scan(&extType, &serviceRef)
    if err != nil {
        return nil, err
    }

    var extensionNode ExtensionNode

    switch extType {
    case "queue":
        // Ví dụ mapping queue đơn giản với callcenter
        extensionNode = ExtensionNode{
            Name: fmt.Sprintf("queue_%s", callee),
            Condition: []ConditionNode{
                {
                    Field: "destination_number",
                    Expr:  fmt.Sprintf("^%s$", callee),
                    Action: []ActionNode{
                        {App: "answer", Data: ""},
                        {App: "set", Data: "queue_name=" + serviceRef},
                        {App: "callcenter", Data: "${queue_name}"},
                    },
                },
            },
        }
    case "ivr":
        extensionNode = ExtensionNode{
            Name: fmt.Sprintf("ivr_%s", callee),
            Condition: []ConditionNode{
                {
                    Field: "destination_number",
                    Expr:  fmt.Sprintf("^%s$", callee),
                    Action: []ActionNode{
                        {App: "answer", Data: ""},
                        {App: "ivr", Data: serviceRef},
                    },
                },
            },
        }
    default:
        // Mặc định: gọi thẳng tới user/extension
        extensionNode = ExtensionNode{
            Name: fmt.Sprintf("user_%s", callee),
            Condition: []ConditionNode{
                {
                    Field: "destination_number",
                    Expr:  fmt.Sprintf("^%s$", callee),
                    Action: []ActionNode{
                        {App: "bridge", Data: "user/" + callee},
                    },
                },
            },
        }
    }

    doc := &Document{
        Type: "freeswitch/xml",
        Section: []Section{
            {
                Name: "dialplan",
                Context: &ContextNode{
                    Name:      contextName,
                    Extension: []ExtensionNode{extensionNode},
                },
            },
        },
    }

    return doc, nil
}
