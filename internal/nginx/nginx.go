package nginx

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	hostRe     = regexp.MustCompile(`^[a-z0-9.-]+$`)
	bindRe     = regexp.MustCompile(`^127\.0\.0\.1:[0-9]+$`)
	hostBindRe = regexp.MustCompile(`^[0-9]{1,3}(?:\.[0-9]{1,3}){3}:[0-9]+$`)
)

// Block is one virtual host. A private block (phpMyAdmin) only listens on 127.0.0.1.
// ErrorLog, when set, is that project's Nginx log and the PHP error_log for its requests.
type Block struct {
	ServerName string
	Root       string
	FastCGI    string
	Private    bool
	ErrorLog   string
}

// Input is a complete Nginx config.
type Input struct {
	Listen    string
	SSLListen string
	// HostListen and HostSSLListen are the Settings host IP, such as 192.168.1.100:80. Empty means 127.0.0.1 only.
	HostListen    string
	HostSSLListen string
	SSLCert       string
	SSLKey        string
	PID           string
	ErrorLog      string
	AccessLog     string
	TempDir       string
	Blocks        []Block
}

// Generate writes an Nginx config that listens on 127.0.0.1, and on the host IP for project blocks.
func Generate(in Input) (string, error) {
	if !bindRe.MatchString(in.Listen) {
		return "", fmt.Errorf("invalid listen address")
	}
	if in.SSLListen != "" {
		if !bindRe.MatchString(in.SSLListen) {
			return "", fmt.Errorf("invalid SSL listen address")
		}
		if in.SSLCert == "" || in.SSLKey == "" {
			return "", fmt.Errorf("SSL certificate paths are required")
		}
	}
	if in.HostListen != "" && !validHostBind(in.HostListen) {
		return "", fmt.Errorf("invalid host IP listen address %s", in.HostListen)
	}
	if in.HostSSLListen != "" && (in.SSLListen == "" || in.HostListen == "" || !validHostBind(in.HostSSLListen)) {
		return "", fmt.Errorf("invalid host IP SSL listen address %s", in.HostSSLListen)
	}
	var b strings.Builder
	pid, err := quote(in.PID)
	if err != nil {
		return "", err
	}
	errorLog, err := quote(in.ErrorLog)
	if err != nil {
		return "", err
	}
	accessLog, err := quote(in.AccessLog)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "worker_processes 1;\n")
	fmt.Fprintf(&b, "pid %s;\n", pid)
	fmt.Fprintf(&b, "error_log %s;\n", errorLog)
	b.WriteString("events {\n    worker_connections 1024;\n}\n")
	b.WriteString("http {\n")
	b.WriteString(mimeTypes)
	b.WriteString("    sendfile on;\n")
	b.WriteString("    charset utf-8;\n")
	if in.SSLListen != "" {
		b.WriteString("    ssl_protocols TLSv1.2 TLSv1.3;\n")
		b.WriteString("    ssl_session_cache shared:SSL:1m;\n")
		b.WriteString("    ssl_session_timeout 10m;\n")
	}
	for _, sub := range []string{"client_body", "proxy", "fastcgi", "uwsgi", "scgi"} {
		path, err := quote(filepath.Join(in.TempDir, sub))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "    %s_temp_path %s;\n", sub, path)
	}
	fmt.Fprintf(&b, "    access_log %s;\n", accessLog)
	b.WriteString("    server {\n")
	if err := writeListen(&b, in, true, false); err != nil {
		return "", err
	}
	b.WriteString("        server_name _;\n        return 404;\n    }\n")
	for _, block := range in.Blocks {
		if err := writeServer(&b, in, block); err != nil {
			return "", err
		}
	}
	b.WriteString("}\n")
	return b.String(), nil
}

func validHostBind(bind string) bool {
	if !hostBindRe.MatchString(bind) {
		return false
	}
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && !ip.IsUnspecified()
}

func writeListen(b *strings.Builder, in Input, defaultServer, private bool) error {
	suffix := ""
	if defaultServer {
		suffix = " default_server"
	}
	host := in.HostListen != "" && !private
	fmt.Fprintf(b, "        listen %s%s;\n", in.Listen, suffix)
	if host {
		fmt.Fprintf(b, "        listen %s%s;\n", in.HostListen, suffix)
	}
	if in.SSLListen == "" {
		return nil
	}
	fmt.Fprintf(b, "        listen %s ssl%s;\n", in.SSLListen, suffix)
	if host && in.HostSSLListen != "" {
		fmt.Fprintf(b, "        listen %s ssl%s;\n", in.HostSSLListen, suffix)
	}
	cert, err := quote(in.SSLCert)
	if err != nil {
		return err
	}
	key, err := quote(in.SSLKey)
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "        ssl_certificate %s;\n", cert)
	fmt.Fprintf(b, "        ssl_certificate_key %s;\n", key)
	return nil
}

func writeServer(b *strings.Builder, in Input, block Block) error {
	if !hostRe.MatchString(block.ServerName) {
		return fmt.Errorf("invalid server name %s", block.ServerName)
	}
	if !bindRe.MatchString(block.FastCGI) {
		return fmt.Errorf("invalid fastcgi address %s", block.FastCGI)
	}
	root, err := quote(block.Root)
	if err != nil {
		return err
	}
	b.WriteString("    server {\n")
	if err := writeListen(b, in, false, block.Private); err != nil {
		return err
	}
	fmt.Fprintf(b, "        server_name %s;\n        root %s;\n        index index.php index.html;\n", block.ServerName, root)
	if block.ErrorLog != "" {
		logged, err := quote(block.ErrorLog)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "        error_log %s;\n", logged)
	}
	if block.Private {
		b.WriteString("        client_max_body_size 64m;\n")
		b.WriteString("        location ~ ^/(libraries|setup|templates|tmp|vendor|config\\.inc\\.php) {\n            deny all;\n        }\n")
	}
	b.WriteString("        location / {\n            try_files $uri $uri/ /index.php?$query_string;\n        }\n")
	fmt.Fprintf(b, "        location ~ \\.php$ {\n            try_files $uri =404;\n            fastcgi_pass %s;\n            fastcgi_index index.php;\n            fastcgi_read_timeout 300s;\n", block.FastCGI)
	for _, param := range fastcgiParams {
		fmt.Fprintf(b, "            fastcgi_param %s;\n", param)
	}
	if block.ErrorLog != "" {
		for _, value := range []string{"log_errors=1", "error_log=" + filepath.ToSlash(block.ErrorLog)} {
			quoted, err := quote(value)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "            fastcgi_param PHP_VALUE %s;\n", quoted)
		}
	}
	b.WriteString("        }\n    }\n")
	return nil
}

func quote(path string) (string, error) {
	if path == "" || strings.ContainsAny(path, "$\n\r") {
		return "", fmt.Errorf("unsupported path %q", path)
	}
	return strconv.Quote(filepath.ToSlash(path)), nil
}

var fastcgiParams = []string{
	"SCRIPT_FILENAME $document_root$fastcgi_script_name",
	"QUERY_STRING $query_string",
	"REQUEST_METHOD $request_method",
	"CONTENT_TYPE $content_type",
	"CONTENT_LENGTH $content_length",
	"SCRIPT_NAME $fastcgi_script_name",
	"REQUEST_URI $request_uri",
	"DOCUMENT_URI $document_uri",
	"DOCUMENT_ROOT $document_root",
	"SERVER_PROTOCOL $server_protocol",
	"GATEWAY_INTERFACE CGI/1.1",
	"SERVER_SOFTWARE nginx",
	"REMOTE_ADDR $remote_addr",
	"REMOTE_PORT $remote_port",
	"SERVER_ADDR $server_addr",
	"SERVER_PORT $server_port",
	"SERVER_NAME $server_name",
	"REDIRECT_STATUS 200",
}

const mimeTypes = `    types {
        text/html html htm;
        text/css css;
        text/plain txt;
        text/xml xml;
        application/javascript js;
        application/json json;
        application/pdf pdf;
        application/wasm wasm;
        application/zip zip;
        image/gif gif;
        image/jpeg jpg jpeg;
        image/png png;
        image/svg+xml svg;
        image/webp webp;
        font/woff woff;
        font/woff2 woff2;
    }
    default_type application/octet-stream;
`
