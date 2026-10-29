package wechat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"io"
	"net/url"
	"time"
)

type Client struct {
	AppID     string
	AppSecret string
	// BaseURL 默认 https://api.weixin.qq.com；单测靠它指向 httptest 假网关。
	// 硬编码上游地址的代码是测不了的——这也是上一版没有任何测试的原因。
	BaseURL string
	client  *http.Client
}

func NewClient(appID, appSecret string) *Client {
	return &Client{
		AppID:     appID,
		AppSecret: appSecret,
		BaseURL:   "https://api.weixin.qq.com",
		client:    &http.Client{Timeout: 5 * time.Second},
	}
}

type Session struct {
	OpenID      string `json:"openid"`
	AccessToken string `json:"access_token"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

func (c *Client) Code2Session(code string) (*Session, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://api.weixin.qq.com"
	}
	u, err := url.Parse(base + "/sns/oauth2/access_token")
	if err != nil {
		return nil, fmt.Errorf("wechat: bad base url %q: %w", base, err)
	}
	// 必须走 url.Values 转义：微信 code 里可能出现 + / = 等字符，
	// 直接拼字符串会把参数截断或注入额外参数。
	q := u.Query()
	q.Set("appid", c.AppID)
	q.Set("secret", c.AppSecret)
	q.Set("code", code)
	q.Set("grant_type", "authorization_code")
	u.RawQuery = q.Encode()

	resp, err := c.client.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wechat: read body: %w", err)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("wechat: upstream %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var s Session
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, err
	}
	if s.ErrCode != 0 {
		return nil, &ErrCodeError{ErrCode: s.ErrCode, ErrMsg: s.ErrMsg}
	}
	return &s, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

type ErrCodeError struct {
	ErrCode int
	ErrMsg  string
}

func (e *ErrCodeError) Error() string {
	return fmt.Sprintf("wechat errcode %d: %s", e.ErrCode, e.ErrMsg)
}
