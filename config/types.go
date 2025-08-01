package config

type config struct {
	Mysql           mysql           `yaml:"mysql" mapstructure:"mysql"`
	CommentSharding commentSharding `yaml:"comment_sharding" mapstructure:"comment_sharding"`
	Redis           redis           `yaml:"redis" mapstructure:"redis"`
	Etcd            etcd            `yaml:"etcd" mapstructure:"etcd"`
	RabbitMq        rabbitmq        `yaml:"rabbitmq" mapstructure:"rabbitmq"`
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
