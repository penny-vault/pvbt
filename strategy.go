package main

type field struct {
	name       string
	help       string
	typecode   string
	options    []string
	defaultval string
}

type strategy interface {
	name() string
	description() string
	fields() []field
	compute() bool
}
