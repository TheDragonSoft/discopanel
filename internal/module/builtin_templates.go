package module

import (
	"context"

	storage "github.com/nickheyer/discopanel/internal/db"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
)

// InitBuiltinTemplates creates/updates built-in module templates
// Only includes templates with real, working Docker images
func InitBuiltinTemplates(store *storage.Store) error {
	ctx := context.Background()

	templates := []storage.ModuleTemplate{
		{
			ID:             "builtin-geyser",
			Name:           "Geyser",
			Description:    "Allows Bedrock Edition players to join Java Edition servers. Requires Floodgate plugin on the server for seamless authentication.",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "nickheyer/discopanel-geyser:latest",
			Category:       "proxy",
			SupportsProxy:  true,
			RequiresServer: true,
			Icon:           "users",
			Ports: []*v1.ModulePort{
				{Name: "Bedrock", ContainerPort: 19132, HostPort: 0, Protocol: "udp", ProxyEnabled: true},
			},
			DefaultAccessUrls: []string{"http://{{host.hostname}}:{{module.ports.Bedrock.host_port}}"},
			DefaultEnv: `{
				"PUID": "{{host.uid}}",
				"PGID": "{{host.gid}}",
				"OVERWRITE_CONFIG": "false",
				"BEDROCK_ADDRESS": "0.0.0.0",
				"BEDROCK_PORT": "{{module.ports.Bedrock.container_port}}",
				"BEDROCK_MOTD1": "GeyserMC",
				"BEDROCK_MOTD2": "Minecraft Server",
				"BEDROCK_SERVERNAME": "Geyser",
				"REMOTE_ADDRESS": "discopanel-server-{{server.id}}",
				"REMOTE_PORT": "25565",
				"REMOTE_AUTH_TYPE": "offline"
			}`,
			DefaultVolumes:  `[{"source": "{{server.data_path}}/modules/geyser", "target": "/data", "read_only": false}]`,
			Documentation:   "Geyser acts as a proxy, translating Bedrock packets to Java packets.",
			HealthCheckPort: 19132,
			DefaultMemory:   1024,
		},
		{
			ID:             "builtin-mc-backup",
			Name:           "MC Backup",
			Description:    "Automated backup solution for Minecraft server worlds with RCON-coordinated saves and configurable retention policies",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "itzg/mc-backup:latest",
			Category:       "utilities",
			SupportsProxy:  false,
			RequiresServer: true,
			Icon:           "archive",
			Ports:          []*v1.ModulePort{},
			DefaultEnv: `{
				"RCON_HOST": "discopanel-server-{{server.id}}",
				"RCON_PORT": "{{server.config.rconPort}}",
				"RCON_PASSWORD": "{{server.config.rconPassword}}",
				"SRC_DIR": "/data",
				"DEST_DIR": "/backups",
				"BACKUP_NAME": "world",
				"BACKUP_METHOD": "tar",
				"BACKUP_INTERVAL": "24h",
				"INITIAL_DELAY": "2m",
				"BACKUP_ON_STARTUP": "true",
				"PRUNE_BACKUPS_DAYS": "7",
				"PAUSE_IF_NO_PLAYERS": "false",
				"EXCLUDES": "*.jar,cache,logs,*.tmp",
				"TZ": "{{server.config.tz}}"
			}`,
			DefaultVolumes: `[
				{"source": "{{server.data_path}}", "target": "/data", "read_only": true},
				{"source": "{{config.storage.backup_dir}}", "target": "/backups", "read_only": false}
				]`,
			Documentation: "Coordinates backups with the Minecraft server via RCON. Automatically flushes data, pauses writes, and resumes after backup. RCON settings are pulled from server config. Backups stored in global backup directory.",
			DefaultMemory: 256,
		},
		{
			ID:             "builtin-rcon-web",
			Name:           "RCON Web Admin",
			Description:    "Web-based RCON client for remote server management with command history and multi-server support",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "itzg/rcon:latest",
			Category:       "management",
			SupportsProxy:  true,
			RequiresServer: true,
			Icon:           "terminal",
			Ports: []*v1.ModulePort{
				{Name: "Web", ContainerPort: 4326, HostPort: 0, Protocol: "http", ProxyEnabled: true},
				{Name: "WS", ContainerPort: 4327, HostPort: 0, Protocol: "http", ProxyEnabled: true},
			},
			DefaultAccessUrls: []string{"http://{{host.hostname}}:{{module.ports.Web.host_port}}"},
			DefaultEnv: `{
				"RWA_ADMIN": "true",
				"RWA_PASSWORD": "admin",
				"RWA_RCON_HOST": "discopanel-server-{{server.id}}",
				"RWA_RCON_PORT": "{{server.config.rconPort}}",
				"RWA_RCON_PASSWORD": "{{server.config.rconPassword}}",
				"RWA_WEBSOCKET_URL": "ws://{{server.proxy_hostname}}:{{module.ports.WS.host_port}}"
			}`,
			DefaultVolumes:  `[]`,
			HealthCheckPath: "/",
			HealthCheckPort: 4326,
			Documentation:   "Provides a web interface for RCON commands. RCON settings are pulled from server config. Web UI on port 4326, WebSocket on port 4327 - both need to be accessible.",
			DefaultMemory:   256,
		},
		{
			ID:             "builtin-minecraft-exporter",
			Name:           "Prometheus Exporter",
			Description:    "Exports Minecraft server metrics to Prometheus for monitoring dashboards",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "itzg/mc-monitor:latest",
			Category:       "monitoring",
			SupportsProxy:  true,
			RequiresServer: true,
			Icon:           "chart-bar",
			DefaultCmd:     "export-for-prometheus",
			Ports: []*v1.ModulePort{
				{Name: "Metrics", ContainerPort: 9225, HostPort: 0, Protocol: "http", ProxyEnabled: true},
			},
			DefaultAccessUrls: []string{"http://{{host.hostname}}:{{module.ports.Metrics.host_port}}/metrics"},
			DefaultEnv: `{
				"EXPORT_SERVERS": "discopanel-server-{{server.id}}:25565",
				"EXPORT_PORT": "{{module.ports.Metrics.container_port}}"
			}`,
			DefaultVolumes:  `[]`,
			HealthCheckPath: "/metrics",
			HealthCheckPort: 9225,
			Documentation:   "Exports server status, player count, TPS, and other metrics in Prometheus format. Connect to /metrics endpoint to scrape metrics.",
			DefaultMemory:   512,
		},
		{
			ID:             "builtin-bluemap",
			Name:           "BlueMap",
			Description:    "Interactive 3D map renderer for Minecraft worlds with a web-based viewer. Renders overworld, nether, and end dimensions.",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "ghcr.io/bluemap-minecraft/bluemap:latest",
			Category:       "monitoring",
			SupportsProxy:  true,
			RequiresServer: true,
			Icon:           "map",
			DefaultCmd:     "-r -u -w",
			Ports: []*v1.ModulePort{
				{Name: "Web", ContainerPort: 8100, HostPort: 0, Protocol: "http", ProxyEnabled: true},
			},
			DefaultAccessUrls: []string{"http://{{host.hostname}}:{{module.ports.Web.host_port}}"},
			DefaultEnv:        `{}`,
			DefaultVolumes: `[
				{"source": "{{server.data_path}}/modules/bluemap/config", "target": "/app/config", "read_only": false, "create_dir": true},
				{"source": "{{server.data_path}}/world", "target": "/app/world", "read_only": true},
				{"source": "{{server.data_path}}/modules/bluemap/data", "target": "/app/data", "read_only": false, "create_dir": true},
				{"source": "{{server.data_path}}/modules/bluemap/web", "target": "/app/web", "read_only": false, "create_dir": true}
				]`,
			HealthCheckPath:         "/",
			HealthCheckPort:         8100,
			Documentation:           "Renders 3D maps of your Minecraft worlds accessible via a web interface. Supports overworld, nether, and end dimensions. World volumes are mounted read-only from the server data path. Config, data, and web assets are stored in the bluemap module directory.",
			DefaultMemory:           2048,
			DefaultUID:              "{{host.uid}}",
			DefaultGID:              "{{host.gid}}",
			DefaultInitCommand:      `sed -i 's/accept-download: false/accept-download: true/' /app/config/core.conf`,
			DefaultInitCommandDelay: 1,
			DefaultRestartAfterInit: true,
		},
		{
			ID:             "builtin-status-panel",
			Name:           "Status Panel",
			Description:    "Real-time server status dashboard showing player count, TPS, memory usage, and server info via the DiscoPanel API.",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "nickheyer/discopanel-status:latest",
			Category:       "monitoring",
			SupportsProxy:  true,
			RequiresServer: true,
			Icon:           "monitor",
			Ports: []*v1.ModulePort{
				{Name: "Web", ContainerPort: 8181, HostPort: 0, Protocol: "http", ProxyEnabled: true},
			},
			DefaultAccessUrls: []string{"http://{{host.hostname}}:{{module.ports.Web.host_port}}"},
			DefaultEnv: `{
				"DISCOPANEL_URL": "http://host.docker.internal:{{config.server.port}}",
				"POLL_INTERVAL": "10s",
				"PORT": "{{module.ports.Web.container_port}}"
			}`,
			DefaultVolumes:  `[]`,
			HealthCheckPath: "/health",
			HealthCheckPort: 8181,
			Documentation:   "Displays a real-time status dashboard for the attached Minecraft server. Fetches status via the DiscoPanel API including player count, TPS, CPU/memory usage, and server configuration. Automatically refreshes every 10 seconds.",
			DefaultMemory:   512,
		},
		{
			ID:             "builtin-playit",
			Name:           "Playit.gg",
			Description:    "Publish this server through a free playit.gg tunnel. Players join via your tunnel's public address, no port forwarding or public IP needed.",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "ghcr.io/discohaus/discomodule-playit:latest",
			Category:       "proxy",
			SupportsProxy:  false,
			RequiresServer: true,
			Icon:           "globe",
			Ports: []*v1.ModulePort{
				{Name: "Status", ContainerPort: 8201, HostPort: 0, Protocol: "http", ProxyEnabled: false},
			},
			DefaultAccessUrls: []string{"https://playit.gg/account/tunnels"},
			DefaultEnv: `{
				"SECRET_KEY": "",
				"TARGET_HOSTNAME": "{{server.proxy_hostname}}",
				"PROXY_PORT": "{{server.proxy_port}}",
				"PROXY_PORT_DEFAULT": "{{config.proxy.listen_port}}",
				"LISTEN_PORT": "25565",
				"UDP_FORWARD": "true",
				"VOICE_PORT": "24454"
			}`,
			DefaultVolumes:  `[{"source": "{{server.data_path}}/modules/playit", "target": "/data", "read_only": false, "create_dir": true}]`,
			HealthCheckPath: "/health",
			HealthCheckPort: 8201,
			Documentation:   "Runs the official playit.gg agent next to a gateway that rewrites incoming Minecraft handshakes onto this server's proxy hostname and relays them into the DiscoPanel proxy, keeping wake-on-connect working. Generate an agent secret key on playit.gg (under Agents) and set it as the SECRET_KEY environment variable. DiscoPanel handles the rest automatically: a matching Minecraft Java tunnel for LISTEN_PORT is created on your account if it doesn't exist yet (disable via module.playit_auto_create), and the assigned public address is captured and shown as the server's connection address on the dashboard. The provisioned secret persists in the module data volume. UDP tunnels for voice mods forward straight to the server container when UDP_FORWARD is on.",
			DefaultMemory:   256,
		},
		{
			ID:             "builtin-grafana",
			Name:           "Grafana",
			Description:    "Observability dashboards for server metrics. Pairs with the Prometheus Exporter module to visualize player counts, TPS, and performance data.",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "grafana/grafana:latest",
			Category:       "monitoring",
			SupportsProxy:  true,
			RequiresServer: false,
			Icon:           "chart-line",
			Ports: []*v1.ModulePort{
				{Name: "Web", ContainerPort: 3000, HostPort: 0, Protocol: "http", ProxyEnabled: true},
			},
			DefaultAccessUrls: []string{"http://{{host.hostname}}:{{module.ports.Web.host_port}}"},
			DefaultEnv: `{
				"GF_SECURITY_ADMIN_PASSWORD": "",
				"GF_USERS_ALLOW_SIGN_UP": "false"
			}`,
			DefaultVolumes:  `[{"source": "{{server.data_path}}/modules/grafana", "target": "/var/lib/grafana", "read_only": false, "create_dir": true}]`,
			HealthCheckPath: "/api/health",
			HealthCheckPort: 3000,
			Documentation:   "Grafana dashboards for your Minecraft metrics. Set the GF_SECURITY_ADMIN_PASSWORD environment variable before starting (admin password; blank default refuses to boot in newer images). Pairs with the Prometheus Exporter module: add a Prometheus datasource in Grafana pointing at the exporter module's container name, e.g. http://discopanel-module-<exporter-module-id>:9225/metrics (the container name is the DNS name on the shared Docker network). Dashboards and data sources persist in the grafana module data directory.",
			DefaultMemory:   512,
			DefaultUID:      "{{host.uid}}",
			DefaultGID:      "{{host.gid}}",
		},
		{
			ID:             "builtin-uptime-kuma",
			Name:           "Uptime Kuma",
			Description:    "Self-hosted uptime monitoring with status pages, alerting, and push/HTTP/TCP monitors for your servers and modules.",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "louislam/uptime-kuma:latest",
			Category:       "monitoring",
			SupportsProxy:  true,
			RequiresServer: false,
			Icon:           "activity",
			Ports: []*v1.ModulePort{
				{Name: "Web", ContainerPort: 3001, HostPort: 0, Protocol: "http", ProxyEnabled: true},
			},
			DefaultAccessUrls: []string{"http://{{host.hostname}}:{{module.ports.Web.host_port}}"},
			DefaultEnv:        `{}`,
			DefaultVolumes:    `[{"source": "{{server.data_path}}/modules/uptime-kuma", "target": "/app/data", "read_only": false, "create_dir": true}]`,
			HealthCheckPath:   "/",
			HealthCheckPort:   3001,
			Documentation:     "Self-hosted uptime monitoring. Create an admin account on first launch, then add monitors for your Minecraft servers (TCP monitor on the server's port) and modules (HTTP monitors on their web ports). Status pages and monitor history persist in the uptime-kuma module data directory. Can also send alerts to other modules such as ntfy.",
			DefaultMemory:     512,
		},
		{
			ID:             "builtin-ntfy",
			Name:           "ntfy",
			Description:    "Self-hosted push notification service over HTTP. Send alerts from DiscoPanel webhooks or other modules to your phone or desktop.",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "binwiederhier/ntfy:latest",
			Category:       "notifications",
			SupportsProxy:  true,
			RequiresServer: false,
			Icon:           "bell",
			DefaultCmd:     "serve",
			Ports: []*v1.ModulePort{
				{Name: "Web", ContainerPort: 80, HostPort: 0, Protocol: "http", ProxyEnabled: true},
			},
			DefaultAccessUrls: []string{"http://{{host.hostname}}:{{module.ports.Web.host_port}}"},
			DefaultEnv:        `{}`,
			DefaultVolumes: `[
				{"source": "{{server.data_path}}/modules/ntfy/cache", "target": "/var/cache/ntfy", "read_only": false, "create_dir": true},
				{"source": "{{server.data_path}}/modules/ntfy/config", "target": "/etc/ntfy", "read_only": false, "create_dir": true}
				]`,
			HealthCheckPath: "/",
			HealthCheckPort: 80,
			Documentation:   "Self-hosted push notifications (runs 'ntfy serve'). Subscribe to topics from the ntfy mobile/desktop apps or the web UI, then use this module as a webhook target for DiscoPanel alert webhooks: POST to http://<this-module-container-name>/my-topic (the container name is the DNS name on the shared Docker network) with the message as the body. Cache and configuration persist in the ntfy module data directory.",
			DefaultMemory:   256,
		},
		{
			ID:             "builtin-mariadb",
			Name:           "MariaDB",
			Description:    "MariaDB database server for Minecraft plugins that need persistent SQL storage. Other modules can reference it via {{deps.mariadb.*}} aliases.",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "mariadb:11",
			Category:       "database",
			SupportsProxy:  false,
			RequiresServer: true,
			Provides:       "mariadb",
			Icon:           "database",
			Ports: []*v1.ModulePort{
				{Name: "DB", ContainerPort: 3306, HostPort: 0, Protocol: "tcp", ProxyEnabled: false},
			},
			DefaultAccessUrls: []string{},
			DefaultEnv: `{
				"MARIADB_ROOT_PASSWORD": "",
				"MARIADB_DATABASE": "minecraft",
				"MARIADB_USER": "minecraft",
				"MARIADB_PASSWORD": ""
			}`,
			DefaultVolumes: `[{"source": "{{server.data_path}}/modules/mariadb", "target": "/var/lib/mysql", "read_only": false, "create_dir": true}]`,
			Documentation:  "MariaDB 11 database server. Set MARIADB_ROOT_PASSWORD and MARIADB_PASSWORD before starting (blank defaults fail to initialize). Point plugins at it with host = this module's container name (discopanel-module-<module-id>, the DNS name on the shared Docker network), port 3306, user/password from the environment variables above, and database 'minecraft'. Dependent modules can reference it via dependency aliases: {{deps.mariadb.host}} for the hostname and {{deps.mariadb.port_DB}} for the port.",
			DefaultMemory:  512,
		},
		{
			ID:             "builtin-redis",
			Name:           "Redis",
			Description:    "Redis in-memory data store for plugins that need fast caching or pub/sub. Other modules can reference it via {{deps.redis.*}} aliases.",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "redis:7-alpine",
			Category:       "database",
			SupportsProxy:  false,
			RequiresServer: true,
			Provides:       "redis",
			Icon:           "database",
			DefaultCmd:     "redis-server --appendonly yes",
			Ports: []*v1.ModulePort{
				{Name: "DB", ContainerPort: 6379, HostPort: 0, Protocol: "tcp", ProxyEnabled: false},
			},
			DefaultAccessUrls: []string{},
			DefaultEnv:        `{}`,
			DefaultVolumes:    `[{"source": "{{server.data_path}}/modules/redis", "target": "/data", "read_only": false, "create_dir": true}]`,
			Documentation:     "Redis 7 with AOF persistence enabled (runs 'redis-server --appendonly yes'). Point plugins at it with host = this module's container name (discopanel-module-<module-id>, the DNS name on the shared Docker network) and port 6379. Dependent modules can reference it via dependency aliases: {{deps.redis.host}} for the hostname and {{deps.redis.port_DB}} for the port. Data persists in the redis module data directory.",
			DefaultMemory:     256,
		},
		{
			ID:             "builtin-postgresql",
			Name:           "PostgreSQL",
			Description:    "PostgreSQL database server for Minecraft plugins that need persistent SQL storage. Other modules can reference it via {{deps.postgres.*}} aliases.",
			Type:           storage.ModuleTemplateTypeBuiltin,
			DockerImage:    "postgres:16-alpine",
			Category:       "database",
			SupportsProxy:  false,
			RequiresServer: true,
			Provides:       "postgres",
			Icon:           "database",
			Ports: []*v1.ModulePort{
				{Name: "DB", ContainerPort: 5432, HostPort: 0, Protocol: "tcp", ProxyEnabled: false},
			},
			DefaultAccessUrls: []string{},
			DefaultEnv: `{
				"POSTGRES_PASSWORD": "",
				"POSTGRES_USER": "minecraft",
				"POSTGRES_DB": "minecraft"
			}`,
			DefaultVolumes: `[{"source": "{{server.data_path}}/modules/postgresql", "target": "/var/lib/postgresql/data", "read_only": false, "create_dir": true}]`,
			Documentation:  "PostgreSQL 16 database server. Set POSTGRES_PASSWORD before starting (blank default fails to initialize). Point plugins at it with host = this module's container name (discopanel-module-<module-id>, the DNS name on the shared Docker network), port 5432, user 'minecraft', password from POSTGRES_PASSWORD, and database 'minecraft'. Dependent modules can reference it via dependency aliases: {{deps.postgres.host}} for the hostname and {{deps.postgres.port_DB}} for the port.",
			DefaultMemory:  512,
		},
	}

	// Upsert each template
	for _, template := range templates {
		if err := store.UpsertModuleTemplate(ctx, &template); err != nil {
			return err
		}
	}

	return nil
}
