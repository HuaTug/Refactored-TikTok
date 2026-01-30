package elasticsearch

import (
	"context"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// IndexType 索引类型常量
const (
	IndexTypeServiceLog = "service-log"
	IndexTypeErrorLog   = "error-log"
	IndexTypeAccessLog  = "access-log"
	IndexTypeAuditLog   = "audit-log"
	IndexTypeAlertLog   = "alert-log"
)

// InitLogIndexTemplates 初始化日志索引模板
func (c *Client) InitLogIndexTemplates(ctx context.Context) error {
	templates := map[string]IndexTemplate{
		"service-log-template": createServiceLogTemplate(c.config.IndexPrefix),
		"error-log-template":   createErrorLogTemplate(c.config.IndexPrefix),
		"access-log-template":  createAccessLogTemplate(c.config.IndexPrefix),
		"audit-log-template":   createAuditLogTemplate(c.config.IndexPrefix),
		"alert-log-template":   createAlertLogTemplate(c.config.IndexPrefix),
	}

	for name, template := range templates {
		if err := c.CreateIndexTemplate(ctx, name, template); err != nil {
			hlog.Errorf("[ES Client] Failed to create template %s: %v", name, err)
			return err
		}
	}

	hlog.Info("[ES Client] All log index templates initialized")
	return nil
}

// createServiceLogTemplate 创建服务日志索引模板
func createServiceLogTemplate(prefix string) IndexTemplate {
	return IndexTemplate{
		IndexPatterns: []string{prefix + "-service-log-*"},
		Priority:      100,
		Template: TemplateSettings{
			Settings: map[string]interface{}{
				"number_of_shards":               3,
				"number_of_replicas":             1,
				"refresh_interval":               "5s",
				"index.lifecycle.name":           "log-retention-policy",
				"index.lifecycle.rollover_alias": prefix + "-service-log",
			},
			Mappings: map[string]interface{}{
				"properties": map[string]interface{}{
					"event_id":       map[string]string{"type": "keyword"},
					"trace_id":       map[string]string{"type": "keyword"},
					"span_id":        map[string]string{"type": "keyword"},
					"parent_span_id": map[string]string{"type": "keyword"},
					"service_name":   map[string]string{"type": "keyword"},
					"method_name":    map[string]string{"type": "keyword"},
					"endpoint":       map[string]string{"type": "keyword"},
					"http_method":    map[string]string{"type": "keyword"},
					"status_code":    map[string]string{"type": "integer"},
					"success":        map[string]string{"type": "boolean"},
					"error_code":     map[string]string{"type": "keyword"},
					"error_message":  map[string]string{"type": "text"},
					"user_id":        map[string]string{"type": "long"},
					"client_ip":      map[string]string{"type": "ip"},
					"user_agent":     map[string]string{"type": "text"},
					"request_size":   map[string]string{"type": "long"},
					"response_size":  map[string]string{"type": "long"},
					"duration":       map[string]string{"type": "long"},
					"timestamp":      map[string]string{"type": "date"},
					"request_body":   map[string]string{"type": "text"},
					"response_body":  map[string]string{"type": "text"},
					"headers":        map[string]string{"type": "object"},
					"extra":          map[string]string{"type": "object"},
					"server_ip":      map[string]string{"type": "ip"},
					"server_host":    map[string]string{"type": "keyword"},
					"environment":    map[string]string{"type": "keyword"},
					"version":        map[string]string{"type": "keyword"},
				},
			},
		},
	}
}

// createErrorLogTemplate 创建错误日志索引模板
func createErrorLogTemplate(prefix string) IndexTemplate {
	return IndexTemplate{
		IndexPatterns: []string{prefix + "-error-log-*"},
		Priority:      100,
		Template: TemplateSettings{
			Settings: map[string]interface{}{
				"number_of_shards":   2,
				"number_of_replicas": 1,
				"refresh_interval":   "5s",
			},
			Mappings: map[string]interface{}{
				"properties": map[string]interface{}{
					"event_id":      map[string]string{"type": "keyword"},
					"trace_id":      map[string]string{"type": "keyword"},
					"service_name":  map[string]string{"type": "keyword"},
					"method_name":   map[string]string{"type": "keyword"},
					"error_code":    map[string]string{"type": "keyword"},
					"error_type":    map[string]string{"type": "keyword"},
					"error_message": map[string]string{"type": "text"},
					"stack_trace":   map[string]string{"type": "text"},
					"level":         map[string]string{"type": "keyword"},
					"user_id":       map[string]string{"type": "long"},
					"client_ip":     map[string]string{"type": "ip"},
					"timestamp":     map[string]string{"type": "date"},
					"context":       map[string]string{"type": "object"},
					"cause":         map[string]string{"type": "text"},
					"server_ip":     map[string]string{"type": "ip"},
					"server_host":   map[string]string{"type": "keyword"},
					"environment":   map[string]string{"type": "keyword"},
					"version":       map[string]string{"type": "keyword"},
				},
			},
		},
	}
}

// createAccessLogTemplate 创建访问日志索引模板
func createAccessLogTemplate(prefix string) IndexTemplate {
	return IndexTemplate{
		IndexPatterns: []string{prefix + "-access-log-*"},
		Priority:      100,
		Template: TemplateSettings{
			Settings: map[string]interface{}{
				"number_of_shards":   3,
				"number_of_replicas": 1,
				"refresh_interval":   "10s",
			},
			Mappings: map[string]interface{}{
				"properties": map[string]interface{}{
					"event_id":      map[string]string{"type": "keyword"},
					"trace_id":      map[string]string{"type": "keyword"},
					"user_id":       map[string]string{"type": "long"},
					"client_ip":     map[string]string{"type": "ip"},
					"endpoint":      map[string]string{"type": "keyword"},
					"http_method":   map[string]string{"type": "keyword"},
					"status_code":   map[string]string{"type": "integer"},
					"duration":      map[string]string{"type": "long"},
					"request_size":  map[string]string{"type": "long"},
					"response_size": map[string]string{"type": "long"},
					"user_agent":    map[string]string{"type": "text"},
					"referer":       map[string]string{"type": "keyword"},
					"timestamp":     map[string]string{"type": "date"},
					"country":       map[string]string{"type": "keyword"},
					"region":        map[string]string{"type": "keyword"},
					"device_type":   map[string]string{"type": "keyword"},
					"platform":      map[string]string{"type": "keyword"},
				},
			},
		},
	}
}

// createAuditLogTemplate 创建审计日志索引模板
func createAuditLogTemplate(prefix string) IndexTemplate {
	return IndexTemplate{
		IndexPatterns: []string{prefix + "-audit-log-*"},
		Priority:      100,
		Template: TemplateSettings{
			Settings: map[string]interface{}{
				"number_of_shards":   2,
				"number_of_replicas": 2, // 审计日志需要更高的可靠性
				"refresh_interval":   "1s",
			},
			Mappings: map[string]interface{}{
				"properties": map[string]interface{}{
					"event_id":      map[string]string{"type": "keyword"},
					"trace_id":      map[string]string{"type": "keyword"},
					"user_id":       map[string]string{"type": "long"},
					"target_id":     map[string]string{"type": "long"},
					"target_type":   map[string]string{"type": "keyword"},
					"action":        map[string]string{"type": "keyword"},
					"resource":      map[string]string{"type": "keyword"},
					"old_value":     map[string]string{"type": "text"},
					"new_value":     map[string]string{"type": "text"},
					"client_ip":     map[string]string{"type": "ip"},
					"user_agent":    map[string]string{"type": "text"},
					"timestamp":     map[string]string{"type": "date"},
					"success":       map[string]string{"type": "boolean"},
					"error_message": map[string]string{"type": "text"},
					"extra":         map[string]string{"type": "object"},
				},
			},
		},
	}
}

// createAlertLogTemplate 创建告警日志索引模板
func createAlertLogTemplate(prefix string) IndexTemplate {
	return IndexTemplate{
		IndexPatterns: []string{prefix + "-alert-log-*"},
		Priority:      100,
		Template: TemplateSettings{
			Settings: map[string]interface{}{
				"number_of_shards":   2,
				"number_of_replicas": 1,
				"refresh_interval":   "1s",
			},
			Mappings: map[string]interface{}{
				"properties": map[string]interface{}{
					"event_id":     map[string]string{"type": "keyword"},
					"alert_id":     map[string]string{"type": "keyword"},
					"alert_name":   map[string]string{"type": "keyword"},
					"alert_type":   map[string]string{"type": "keyword"},
					"severity":     map[string]string{"type": "keyword"},
					"service_name": map[string]string{"type": "keyword"},
					"metric_name":  map[string]string{"type": "keyword"},
					"metric_value": map[string]string{"type": "float"},
					"threshold":    map[string]string{"type": "float"},
					"message":      map[string]string{"type": "text"},
					"timestamp":    map[string]string{"type": "date"},
					"status":       map[string]string{"type": "keyword"},
					"labels":       map[string]string{"type": "object"},
					"annotations":  map[string]string{"type": "object"},
					"environment":  map[string]string{"type": "keyword"},
				},
			},
		},
	}
}
