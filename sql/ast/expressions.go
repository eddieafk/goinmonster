package ast

type Node interface {
	node()
}

type Statement interface {
	Node
	statement()
}

type Expression interface {
	Node
	expression()
}
