package redis

type _Redis struct {
	Addr     string
	PassWord string
	DB       int
}

var Interaction = _Redis{Addr: "localhost:6379", PassWord: "Redis@TikTok2025_SecurePass", DB: 1}
