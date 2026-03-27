package client

import "context"

type unixWorker struct {
	ctx  context.Context
	name string
}
