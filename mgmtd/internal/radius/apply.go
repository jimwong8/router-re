package radius

import "fmt"

// RenderRadiusClientConf generates the contents of radiusclient.conf.
func RenderRadiusClientConf(cfg Config) string {
	port := cfg.port()
	server := cfg.Server
	if server == "" {
		server = "127.0.0.1"
	}
	return fmt.Sprintf(`auth_order radius
login_tries 4
authserver %s:%d
acctserver %s:%d
servers /etc/radiusclient/servers
dictionary /usr/share/radiusclient/dictionary
`, server, port, server, port)
}

// RenderServersFile generates the servers file line: "<server>\t<secret>".
func RenderServersFile(cfg Config) string {
	return fmt.Sprintf("%s\t%s\n", cfg.Server, cfg.Secret)
}
