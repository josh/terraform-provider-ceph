package cephcli

type CLI struct {
	confPath string
}

func New(confPath string) *CLI {
	return &CLI{confPath: confPath}
}
