package config

type config struct {
	Mysql           mysql           `yaml:"mysql" mapstructure:"mysql"`
	CommentSharding commentSharding `yaml:"comment_sharding" mapstructure:"comment_sharding"`
	FollowsSharding FollowsSharding `yaml:"follows_sharding" mapstructure:"follows_sharding"`
	Redis           redis           `yaml:"redis" mapstructure:"redis"`
	Etcd            etcd            `yaml:"etcd" mapstructure:"etcd"`
	RabbitMq        rabbitmq        `yaml:"rabbitmq" mapstructure:"rabbitmq"`
	Kafka           kafka           `yaml:"kafka" mapstructure:"kafka"`
	Elasticsearch   elasticsearch   `yaml:"elasticsearch" mapstructure:"elasticsearch"`
	Ollama          ollamaConfig    `yaml:"ollama" mapstructure:"ollama"`
}

// kafka 配置
type kafka struct {
	Brokers            []string `yaml:"brokers" mapstructure:"brokers"`
	Version            string   `yaml:"version" mapstructure:"version"`
	ProducerRetries    int      `yaml:"producer_retries" mapstructure:"producer_retries"`
	ConsumerOffsetInit string   `yaml:"consumer_offset_init" mapstructure:"consumer_offset_init"` // newest / oldest
}

// elasticsearch 配置
type elasticsearch struct {
	Addresses   []string `yaml:"addresses" mapstructure:"addresses"`       // ES 节点地址列表
	Username    string   `yaml:"username" mapstructure:"username"`         // 用户名
	Password    string   `yaml:"password" mapstructure:"password"`         // 密码
	IndexPrefix string   `yaml:"index_prefix" mapstructure:"index_prefix"` // 索引前缀
	MaxRetries  int      `yaml:"max_retries" mapstructure:"max_retries"`   // 最大重试次数
	EnableSniff bool     `yaml:"enable_sniff" mapstructure:"enable_sniff"` // 是否启用节点嗅探
}

type mysql struct {
	Addr     string `yaml:"addr"`
	Database string `yaml:"database"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Charset  string `yaml:"charset"`
	Params   string `yaml:"params"`
}

type commentSharding struct {
	DatabaseCount   int        `yaml:"database_count" mapstructure:"database_count"`
	TableCount      int        `yaml:"table_count" mapstructure:"table_count"`
	MaxOpenConns    int        `yaml:"max_open_conns" mapstructure:"max_open_conns"`
	MaxIdleConns    int        `yaml:"max_idle_conns" mapstructure:"max_idle_conns"`
	ConnMaxLifetime string     `yaml:"conn_max_lifetime" mapstructure:"conn_max_lifetime"`
	MasterDSNs      []string   `yaml:"master_dsns" mapstructure:"master_dsns"`
	SlaveDSNs       [][]string `yaml:"slave_dsns" mapstructure:"slave_dsns"`
}

type FollowsSharding struct {
	DatabaseCount   int        `yaml:"database_count" mapstructure:"database_count"`
	TableCount      int        `yaml:"table_count" mapstructure:"table_count"`
	MaxOpenConns    int        `yaml:"max_open_conns" mapstructure:"max_open_conns"`
	MaxIdleConns    int        `yaml:"max_idle_conns" mapstructure:"max_idle_conns"`
	ConnMaxLifetime string     `yaml:"conn_max_lifetime" mapstructure:"conn_max_lifetime"`
	MasterDSNs      []string   `yaml:"master_dsns" mapstructure:"master_dsns"`
	SlaveDSNs       [][]string `yaml:"slave_dsns" mapstructure:"slave_dsns"`
}

type redis struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
}
type etcd struct {
	Addr string `yaml:"addr"`
}
type rabbitmq struct {
	Addr     string `yaml:"addr"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Ollama LLM configuration
type ollamaConfig struct {
	BaseURL      string  `yaml:"base_url" mapstructure:"base_url"`           // Ollama API base URL, e.g. http://localhost:11434
	Model        string  `yaml:"model" mapstructure:"model"`                 // Model name, e.g. llama3, qwen2, your custom model
	Temperature  float64 `yaml:"temperature" mapstructure:"temperature"`     // Sampling temperature (0.0 - 2.0)
	MaxTokens    int     `yaml:"max_tokens" mapstructure:"max_tokens"`       // Maximum tokens for response
	Timeout      int     `yaml:"timeout" mapstructure:"timeout"`             // Request timeout in seconds
	SystemPrompt string  `yaml:"system_prompt" mapstructure:"system_prompt"` // Default system prompt
	Enabled      bool    `yaml:"enabled" mapstructure:"enabled"`             // Whether Ollama integration is enabled
}
