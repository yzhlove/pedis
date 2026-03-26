package client

type redisWork struct {
}

func NewRedisWork() Worker {
	return &redisWork{}
}
