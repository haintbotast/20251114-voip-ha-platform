package fsxml

import "encoding/xml"

type Document struct {
    XMLName xml.Name  `xml:"document"`
    Type    string    `xml:"type,attr"`
    Section []Section `xml:"section"`
}

type Section struct {
    Name        string       `xml:"name,attr"`
    Description string       `xml:"description,attr,omitempty"`
    Domain      *DomainNode  `xml:"domain,omitempty"`
    Context     *ContextNode `xml:"context,omitempty"`
}

type DomainNode struct {
    Name string     `xml:"name,attr"`
    User []UserNode `xml:"user"`
}

type UserNode struct {
    ID     string         `xml:"id,attr"`
    Params []ParamNode    `xml:"params>param,omitempty"`
    Vars   []VariableNode `xml:"variables>variable,omitempty"`
}

type ParamNode struct {
    Name  string `xml:"name,attr"`
    Value string `xml:"value,attr"`
}

type VariableNode struct {
    Name  string `xml:"name,attr"`
    Value string `xml:"value,attr"`
}

type ContextNode struct {
    Name      string          `xml:"name,attr"`
    Extension []ExtensionNode `xml:"extension"`
}

type ExtensionNode struct {
    Name      string          `xml:"name,attr"`
    Condition []ConditionNode `xml:"condition"`
}

type ConditionNode struct {
    Field  string       `xml:"field,attr,omitempty"`
    Expr   string       `xml:"expression,attr,omitempty"`
    Action []ActionNode `xml:"action"`
}

type ActionNode struct {
    App  string `xml:"application,attr"`
    Data string `xml:"data,attr"`
}
