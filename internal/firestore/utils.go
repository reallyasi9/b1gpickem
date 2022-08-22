package firestore

import (
	"fmt"
	"strings"

	fs "cloud.google.com/go/firestore"
)

func treeElement(name string, indent int, last bool) string {
	var sb strings.Builder
	sb.WriteString(strings.Repeat(" ", indent))
	if last {
		sb.WriteRune('└')
	} else {
		sb.WriteRune('├')
	}
	sb.WriteString(fmt.Sprintf(" %s", name))
	return sb.String()
}

func pathOrNil(r *fs.DocumentRef) string {
	var sb strings.Builder
	sb.WriteString("→(")
	if r != nil {
		sb.WriteString(r.Path)
	}
	sb.WriteRune(')')
	return sb.String()
}

func treeRef(name string, indent int, last bool, r *fs.DocumentRef) string {
	var sb strings.Builder
	sb.WriteString(treeElement(name, indent, last))
	sb.WriteString(": ")
	sb.WriteString(pathOrNil(r))
	return sb.String()
}

func treeString(name string, indent int, last bool, value string) string {
	var sb strings.Builder
	sb.WriteString(treeElement(name, indent, last))
	sb.WriteString(": ")
	sb.WriteString(value)
	return sb.String()
}

func treeBool(name string, indent int, last bool, value bool) string {
	var sb strings.Builder
	sb.WriteString(treeElement(name, indent, last))
	sb.WriteString(fmt.Sprintf(": %t", value))
	return sb.String()
}

func treeInt(name string, indent int, last bool, value int) string {
	var sb strings.Builder
	sb.WriteString(treeElement(name, indent, last))
	sb.WriteString(fmt.Sprintf(": %d", value))
	return sb.String()
}

func treeFloat64(name string, indent int, last bool, value float64) string {
	var sb strings.Builder
	sb.WriteString(treeElement(name, indent, last))
	sb.WriteString(fmt.Sprintf(": %f", value))
	return sb.String()
}

func treeIntPtr(name string, indent int, last bool, value *int) string {
	var sb strings.Builder
	sb.WriteString(treeElement(name, indent, last))
	sb.WriteRune(':')
	if value != nil {
		sb.WriteString(fmt.Sprintf(" %d", *value))
	}
	return sb.String()
}

func treeStringSlice(name string, indent int, last bool, value []string) string {
	var sb strings.Builder
	sb.WriteString(treeElement(name, indent, last))
	sb.WriteString(fmt.Sprintf(": slice[%d] ↓↓↓", len(value)))
	ss := make([]string, len(value))
	for i, s := range value {
		ss[i] = fmt.Sprintf("│%*d: %s", indent+3, i, s)
	}
	if len(ss) > 0 {
		sb.WriteRune('\n')
		sb.WriteString(strings.Join(ss, "\n"))
	}
	return sb.String()
}

func treeFloat64Slice(name string, indent int, last bool, value []float64) string {
	var sb strings.Builder
	sb.WriteString(treeElement(name, indent, last))
	sb.WriteString(fmt.Sprintf(": slice[%d] ↓↓↓", len(value)))
	ss := make([]string, len(value))
	for i, s := range value {
		ss[i] = fmt.Sprintf("│%*d: %f", indent+3, i, s)
	}
	if len(ss) > 0 {
		sb.WriteRune('\n')
		sb.WriteString(strings.Join(ss, "\n"))
	}
	return sb.String()
}

func treeRefSlice(name string, indent int, last bool, value []*fs.DocumentRef) string {
	var sb strings.Builder
	sb.WriteString(treeElement(name, indent, last))
	sb.WriteString(fmt.Sprintf(": slice[%d] ↓↓↓", len(value)))
	ss := make([]string, len(value))
	for i, s := range value {
		ss[i] = fmt.Sprintf("│%*d: %s", indent+3, i, pathOrNil(s))
	}
	if len(ss) > 0 {
		sb.WriteRune('\n')
		sb.WriteString(strings.Join(ss, "\n"))
	}
	return sb.String()
}

func treeUint64RefMap(name string, indent int, last bool, value map[uint64]*fs.DocumentRef) string {
	var sb strings.Builder
	sb.WriteString(treeElement(name, indent, last))
	sb.WriteString(fmt.Sprintf(": map[%d] ↓↓↓", len(value)))
	ss := make([]string, len(value))
	for i, s := range value {
		ss[i] = fmt.Sprintf("│%*d: %s", indent+3, i, pathOrNil(s))
	}
	if len(ss) > 0 {
		sb.WriteRune('\n')
		sb.WriteString(strings.Join(ss, "\n"))
	}
	return sb.String()
}

func treeStringRefMap(name string, indent int, last bool, value map[string]*fs.DocumentRef) string {
	var sb strings.Builder
	sb.WriteString(treeElement(name, indent, last))
	sb.WriteString(fmt.Sprintf(": map[%d] ↓↓↓", len(value)))
	ss := make([]string, 0, len(value))
	for i, s := range value {
		ss = append(ss, fmt.Sprintf("│%*s%s: %s", indent+3, " ", i, pathOrNil(s)))
	}
	if len(ss) > 0 {
		sb.WriteRune('\n')
		sb.WriteString(strings.Join(ss, "\n"))
	}
	return sb.String()
}
