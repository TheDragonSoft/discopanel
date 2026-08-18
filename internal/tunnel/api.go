package tunnel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type PlayitAPIClient struct {
	httpClient *http.Client
	baseURL    string
}

func NewPlayitAPIClient() *PlayitAPIClient {
	return &PlayitAPIClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    "https://api.playit.gg",
	}
}

type PlayitTunnelDetails struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	TunnelType    string `json:"tunnel_type"`
	PortType      string `json:"port_type"`
	PublicAddress string `json:"public_address"`
	PublicPort    int    `json:"public_port"`
	LocalPort     int    `json:"local_port"`
	Enabled       bool   `json:"enabled"`
}

type playitCreateReq struct {
	Name       string       `json:"name,omitempty"`
	TunnelType string       `json:"tunnel_type,omitempty"`
	PortType   string       `json:"port_type"` // "tcp", "udp", "both"
	PortCount  int          `json:"port_count"`
	Enabled    bool         `json:"enabled"`
	Origin     playitOrigin `json:"origin"`
}

type playitOrigin struct {
	Type string           `json:"type"` // "default"
	Data playitOriginData `json:"data"`
}

type playitOriginData struct {
	LocalIP   string `json:"local_ip"`
	LocalPort int    `json:"local_port,omitempty"`
}

// SetupClaim registers a new agent claim code with Playit.gg
func (c *PlayitAPIClient) SetupClaim(ctx context.Context, code string) error {
	body := map[string]string{
		"code":       code,
		"agent_type": "self-managed",
		"version":    "1.0.10",
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/claim/setup", bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "playit-agent/1.0.10")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("claim setup failed (status %d): %s", resp.StatusCode, string(respBytes))
	}
	return nil
}

// ExchangeClaim attempts to exchange a claim code for an active Playit agent secret key
func (c *PlayitAPIClient) ExchangeClaim(ctx context.Context, code string) (string, error) {
	body := map[string]string{
		"code": code,
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/claim/exchange", bytes.NewReader(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "playit-agent/1.0.10")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res struct {
		Status string `json:"status"`
		Data   struct {
			SecretKey string `json:"secret_key"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	if res.Status == "success" && res.Data.SecretKey != "" {
		return res.Data.SecretKey, nil
	}
	return "", fmt.Errorf("claim code not accepted yet or invalid")
}

// CreateTunnel calls the Playit.gg API to automatically provision a tunnel on the user's account
func (c *PlayitAPIClient) CreateTunnel(ctx context.Context, secretKey string, name string, tunnelType string, portType string, targetPort int) (*PlayitTunnelDetails, error) {
	if strings.TrimSpace(secretKey) == "" {
		return nil, fmt.Errorf("secret key is required")
	}

	if portType == "" {
		portType = "both"
	}
	if portType == "both" && tunnelType == "minecraft-bedrock" {
		portType = "udp"
	}

	reqBody := playitCreateReq{
		Name:       name,
		TunnelType: tunnelType,
		PortType:   portType,
		PortCount:  1,
		Enabled:    true,
		Origin: playitOrigin{
			Type: "default",
			Data: playitOriginData{
				LocalIP:   "127.0.0.1",
				LocalPort: targetPort,
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/tunnels/create", bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}

	c.setHeaders(req, secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("playit api error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// Also list tunnels to extract full allocated address details
	tunnels, listErr := c.ListTunnels(ctx, secretKey)
	if listErr == nil && len(tunnels) > 0 {
		for _, t := range tunnels {
			if t.LocalPort == targetPort {
				return t, nil
			}
		}
	}

	return &PlayitTunnelDetails{
		Name:       name,
		TunnelType: tunnelType,
		PortType:   portType,
		LocalPort:  targetPort,
		Enabled:    true,
	}, nil
}

// DeleteTunnel deletes a tunnel on Playit.gg
func (c *PlayitAPIClient) DeleteTunnel(ctx context.Context, secretKey string, tunnelID string) error {
	if strings.TrimSpace(secretKey) == "" || strings.TrimSpace(tunnelID) == "" {
		return nil
	}

	body := map[string]string{
		"tunnel_id": tunnelID,
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/tunnels/delete", bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	c.setHeaders(req, secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete tunnel from Playit (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}
	return nil
}

// ListTunnels lists all tunnels registered to the given Playit agent secret key
func (c *PlayitAPIClient) ListTunnels(ctx context.Context, secretKey string) ([]*PlayitTunnelDetails, error) {
	if strings.TrimSpace(secretKey) == "" {
		return nil, fmt.Errorf("secret key is required")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/tunnels/list", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}

	c.setHeaders(req, secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	dataMap, ok := res["data"].(map[string]interface{})
	if !ok {
		return []*PlayitTunnelDetails{}, nil
	}

	rawTunnels, ok := dataMap["tunnels"].([]interface{})
	if !ok {
		return []*PlayitTunnelDetails{}, nil
	}

	var results []*PlayitTunnelDetails
	for _, raw := range rawTunnels {
		tmap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		item := &PlayitTunnelDetails{}
		if id, ok := tmap["id"].(string); ok {
			item.ID = id
		}
		if name, ok := tmap["name"].(string); ok {
			item.Name = name
		}
		if portType, ok := tmap["port_type"].(string); ok {
			item.PortType = portType
		}
		if tunnelType, ok := tmap["tunnel_type"].(string); ok {
			item.TunnelType = tunnelType
		}

		// 0. Extract display_address if directly present
		if displayAddr, ok := tmap["display_address"].(string); ok && displayAddr != "" {
			item.PublicAddress = displayAddr
		}

		// 1. Extract domain object
		if domain, ok := tmap["domain"].(map[string]interface{}); ok {
			if domainName, ok := domain["name"].(string); ok && domainName != "" && item.PublicAddress == "" {
				item.PublicAddress = domainName
			}
			if domainStr, ok := domain["domain"].(string); ok && domainStr != "" && item.PublicAddress == "" {
				item.PublicAddress = domainStr
			}
		}

		// 2. Extract allocation object
		if alloc, ok := tmap["alloc"].(map[string]interface{}); ok {
			if allocData, ok := alloc["data"].(map[string]interface{}); ok {
				if assignedDomain, ok := allocData["assigned_domain"].(string); ok && assignedDomain != "" && item.PublicAddress == "" {
					item.PublicAddress = assignedDomain
				}
				if ipHost, ok := allocData["ip_hostname"].(string); ok && ipHost != "" && item.PublicAddress == "" {
					item.PublicAddress = ipHost
				}
				if assignedSrv, ok := allocData["assigned_srv"].(string); ok && assignedSrv != "" && item.PublicAddress == "" {
					item.PublicAddress = assignedSrv
				}
				if port, ok := allocData["port_start"].(float64); ok && port > 0 {
					item.PublicPort = int(port)
				} else if port, ok := allocData["port"].(float64); ok && port > 0 {
					item.PublicPort = int(port)
				}
			}
		}

		// 3a. Extract origin local port
		if origin, ok := tmap["origin"].(map[string]interface{}); ok {
			if originData, ok := origin["data"].(map[string]interface{}); ok {
				if lp, ok := originData["local_port"].(float64); ok && lp > 0 {
					item.LocalPort = int(lp)
				} else if port, ok := originData["port"].(float64); ok && port > 0 {
					item.LocalPort = int(port)
				}
			}
		}

		// 3b. Extract agent_config fields (from /v1/agents/rundata or list)
		if agentConfig, ok := tmap["agent_config"].(map[string]interface{}); ok {
			if fields, ok := agentConfig["fields"].([]interface{}); ok {
				for _, f := range fields {
					if fmap, ok := f.(map[string]interface{}); ok {
						if fmap["name"] == "local_port" {
							if valStr, ok := fmap["value"].(string); ok {
								if p, err := strconv.Atoi(valStr); err == nil && p > 0 {
									item.LocalPort = p
								}
							} else if valNum, ok := fmap["value"].(float64); ok && valNum > 0 {
								item.LocalPort = int(valNum)
							}
						}
					}
				}
			}
		}

		// 4. Default fallback port if unspecified
		if item.LocalPort <= 0 {
			if item.TunnelType == "minecraft-bedrock" {
				item.LocalPort = 19132
			} else {
				item.LocalPort = 25565
			}
		}

		results = append(results, item)
	}

	return results, nil
}

func (c *PlayitAPIClient) setHeaders(req *http.Request, secretKey string) {
	cleanKey := strings.TrimSpace(secretKey)
	req.Header.Set("Content-Type", "application/json")
	if strings.HasPrefix(cleanKey, "agent-key ") {
		req.Header.Set("Authorization", cleanKey)
	} else if strings.HasPrefix(cleanKey, "AgentKey ") {
		req.Header.Set("Authorization", "agent-key "+strings.TrimPrefix(cleanKey, "AgentKey "))
	} else {
		req.Header.Set("Authorization", "agent-key "+cleanKey)
	}
}
