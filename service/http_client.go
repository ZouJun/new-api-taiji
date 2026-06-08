package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"golang.org/x/net/proxy"
)

var (
	httpClient          *http.Client
	baseTransport       *http.Transport
	defaultClientLock   sync.Mutex
	defaultClients      = make(map[int]*http.Client)
	proxyClientLock     sync.Mutex
	proxyClients        = make(map[string]*http.Client)
	proxyTransportCache = make(map[string]*http.Transport)
)

func checkRedirect(req *http.Request, via []*http.Request) error {
	fetchSetting := system_setting.GetFetchSetting()
	urlStr := req.URL.String()
	if err := common.ValidateURLWithFetchSetting(urlStr, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		return fmt.Errorf("redirect to %s blocked: %v", urlStr, err)
	}
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	return nil
}

func InitHttpClient() {
	baseTransport = &http.Transport{
		MaxIdleConns:        common.RelayMaxIdleConns,
		MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
		IdleConnTimeout:     time.Duration(common.RelayIdleConnTimeout) * time.Second,
		ForceAttemptHTTP2:   true,
		Proxy:               http.ProxyFromEnvironment, // Support HTTP_PROXY, HTTPS_PROXY, NO_PROXY env vars
	}
	if common.TLSInsecureSkipVerify {
		baseTransport.TLSClientConfig = common.InsecureTLSConfig
	}

	httpClient = buildHTTPClient(baseTransport, common.RelayTimeout)
	defaultClientLock.Lock()
	defaultClients = map[int]*http.Client{
		common.RelayTimeout: httpClient,
	}
	defaultClientLock.Unlock()
}

func GetHttpClient() *http.Client {
	return httpClient
}

func buildHTTPClient(transport http.RoundTripper, timeoutSeconds int) *http.Client {
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
	if timeoutSeconds > 0 {
		client.Timeout = time.Duration(timeoutSeconds) * time.Second
	}
	return client
}

func GetHttpClientByTimeout(timeoutSeconds int) *http.Client {
	if timeoutSeconds < 0 {
		timeoutSeconds = 0
	}
	if baseTransport == nil {
		InitHttpClient()
	}
	defaultClientLock.Lock()
	defer defaultClientLock.Unlock()
	if client, ok := defaultClients[timeoutSeconds]; ok {
		return client
	}
	client := buildHTTPClient(baseTransport, timeoutSeconds)
	defaultClients[timeoutSeconds] = client
	return client
}

// GetHttpClientWithProxy returns the default client or a proxy-enabled one when proxyURL is provided.
func GetHttpClientWithProxy(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		return GetHttpClient(), nil
	}
	return NewProxyHttpClient(proxyURL)
}

func GetHttpClientWithProxyAndTimeout(proxyURL string, timeoutSeconds int) (*http.Client, error) {
	if proxyURL == "" {
		return GetHttpClientByTimeout(timeoutSeconds), nil
	}
	return NewProxyHttpClient(proxyURL, timeoutSeconds)
}

// ResetProxyClientCache 清空代理客户端缓存，确保下次使用时重新初始化
func ResetProxyClientCache() {
	proxyClientLock.Lock()
	defer proxyClientLock.Unlock()
	for _, client := range proxyClients {
		if transport, ok := client.Transport.(*http.Transport); ok && transport != nil {
			transport.CloseIdleConnections()
		}
	}
	proxyClients = make(map[string]*http.Client)
	proxyTransportCache = make(map[string]*http.Transport)
}

// NewProxyHttpClient 创建支持代理的 HTTP 客户端
func NewProxyHttpClient(proxyURL string, timeoutSeconds ...int) (*http.Client, error) {
	resolvedTimeout := common.RelayTimeout
	if len(timeoutSeconds) > 0 && timeoutSeconds[0] >= 0 {
		resolvedTimeout = timeoutSeconds[0]
	}
	if proxyURL == "" {
		if client := GetHttpClientByTimeout(resolvedTimeout); client != nil {
			return client, nil
		}
		return http.DefaultClient, nil
	}

	cacheKey := fmt.Sprintf("%s|%d", proxyURL, resolvedTimeout)
	proxyClientLock.Lock()
	if client, ok := proxyClients[cacheKey]; ok {
		proxyClientLock.Unlock()
		return client, nil
	}
	proxyClientLock.Unlock()

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	switch parsedURL.Scheme {
	case "http", "https":
		proxyClientLock.Lock()
		transport, ok := proxyTransportCache[proxyURL]
		if !ok {
			transport = &http.Transport{
				MaxIdleConns:        common.RelayMaxIdleConns,
				MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
				IdleConnTimeout:     time.Duration(common.RelayIdleConnTimeout) * time.Second,
				ForceAttemptHTTP2:   true,
				Proxy:               http.ProxyURL(parsedURL),
			}
			if common.TLSInsecureSkipVerify {
				transport.TLSClientConfig = common.InsecureTLSConfig
			}
			proxyTransportCache[proxyURL] = transport
		}
		proxyClientLock.Unlock()

		client := buildHTTPClient(transport, resolvedTimeout)
		proxyClientLock.Lock()
		proxyClients[cacheKey] = client
		proxyClientLock.Unlock()
		return client, nil

	case "socks5", "socks5h":
		// 获取认证信息
		var auth *proxy.Auth
		if parsedURL.User != nil {
			auth = &proxy.Auth{
				User:     parsedURL.User.Username(),
				Password: "",
			}
			if password, ok := parsedURL.User.Password(); ok {
				auth.Password = password
			}
		}

		// 创建 SOCKS5 代理拨号器
		// proxy.SOCKS5 使用 tcp 参数，所有 TCP 连接包括 DNS 查询都将通过代理进行。行为与 socks5h 相同
		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}

		proxyClientLock.Lock()
		transport, ok := proxyTransportCache[proxyURL]
		if !ok {
			transport = &http.Transport{
				MaxIdleConns:        common.RelayMaxIdleConns,
				MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
				IdleConnTimeout:     time.Duration(common.RelayIdleConnTimeout) * time.Second,
				ForceAttemptHTTP2:   true,
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				},
			}
			if common.TLSInsecureSkipVerify {
				transport.TLSClientConfig = common.InsecureTLSConfig
			}
			proxyTransportCache[proxyURL] = transport
		}
		proxyClientLock.Unlock()

		client := buildHTTPClient(transport, resolvedTimeout)
		proxyClientLock.Lock()
		proxyClients[cacheKey] = client
		proxyClientLock.Unlock()
		return client, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s, must be http, https, socks5 or socks5h", parsedURL.Scheme)
	}
}
