package manifest

import "text/template/parse"

// ExtractFieldRefs returns the first identifier of each FieldNode in the parse
// tree. For `{{.host}}:{{.port}}` it returns ["host", "port"]. Nested field
// accesses like `{{.foo.bar}}` yield only the root identifier ("foo").
func ExtractFieldRefs(tree *parse.Tree) []string {
	if tree == nil || tree.Root == nil {
		return nil
	}
	var refs []string
	var walk func(node parse.Node)
	walk = func(node parse.Node) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case *parse.ListNode:
			for _, item := range n.Nodes {
				walk(item)
			}
		case *parse.ActionNode:
			walk(n.Pipe)
		case *parse.PipeNode:
			for _, cmd := range n.Cmds {
				for _, arg := range cmd.Args {
					walk(arg)
				}
			}
		case *parse.FieldNode:
			if len(n.Ident) > 0 {
				refs = append(refs, n.Ident[0])
			}
		case *parse.IfNode:
			walk(n.Pipe)
			walk(n.List)
			walk(n.ElseList)
		case *parse.RangeNode:
			walk(n.Pipe)
			walk(n.List)
			walk(n.ElseList)
		case *parse.WithNode:
			walk(n.Pipe)
			walk(n.List)
			walk(n.ElseList)
		}
	}
	walk(tree.Root)
	return refs
}
