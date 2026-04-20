package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	workingProxies    []string
	proxyMutex        sync.RWMutex
	lastProxyUpdate   time.Time
	currentProxy      string
	currentProxyMutex sync.Mutex
)

func GetProxyCount() int {
	proxyMutex.RLock()
	defer proxyMutex.RUnlock()
	return len(workingProxies)
}

func UpdateWorkingProxies() {
	proxies, err := getProxyList()
	if err != nil {
		fmt.Printf("❌ Failed to get proxy list: %v\n", err)
		return
	}

	fmt.Printf("🔍 Testing %d proxies...\n", len(proxies))

	var wg sync.WaitGroup
	results := make(chan string, len(proxies))
	semaphore := make(chan struct{}, 50)

	for _, proxy := range proxies {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if checkProxy(p) {
				results <- p
			}
		}(proxy)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	newProxies := make([]string, 0, len(proxies)/10)
	for proxy := range results {
		newProxies = append(newProxies, proxy)
	}

	if len(newProxies) > 0 {
		proxyMutex.Lock()
		workingProxies = newProxies
		lastProxyUpdate = time.Now()
		proxyMutex.Unlock()

		currentProxyMutex.Lock()
		if currentProxy != "" {
			stillValid := false
			proxyMutex.RLock()
			for _, p := range workingProxies {
				if p == currentProxy {
					stillValid = true
					break
				}
			}
			proxyMutex.RUnlock()
			if !stillValid {
				currentProxy = ""
			}
		}
		currentProxyMutex.Unlock()

		fmt.Printf("✅ Found %d working proxies out of %d tested\n", len(newProxies), len(proxies))
	} else {
		fmt.Println("⚠️ No working proxies found after testing")
	}
}

func GetWorkingProxy() string {
	currentProxyMutex.Lock()
	defer currentProxyMutex.Unlock()

	if currentProxy != "" {
		proxyMutex.RLock()
		for _, p := range workingProxies {
			if p == currentProxy {
				proxyMutex.RUnlock()
				return currentProxy
			}
		}
		proxyMutex.RUnlock()
		fmt.Printf("⚠️ Current proxy %s no longer in pool, selecting new one\n", currentProxy)
		currentProxy = ""
	}

	proxyMutex.RLock()
	if len(workingProxies) == 0 {
		proxyMutex.RUnlock()
		if time.Since(lastProxyUpdate) > 2*time.Minute {
			fmt.Println("⚠️ No working proxies available, refreshing list...")
			go UpdateWorkingProxies()
		}
		return ""
	}
	currentProxy = workingProxies[0]
	proxyMutex.RUnlock()

	fmt.Printf("📌 Selected new active proxy: %s\n", currentProxy)
	return currentProxy
}

func MarkProxyFailed(proxy string) {
	if proxy == "" {
		return
	}

	currentProxyMutex.Lock()
	if currentProxy == proxy {
		fmt.Printf("❌ Marking current proxy as failed: %s\n", proxy)
		currentProxy = ""
	}
	currentProxyMutex.Unlock()

	RemoveProxy(proxy)
}

func GetNextProxy() string {
	proxyMutex.RLock()
	defer proxyMutex.RUnlock()

	if len(workingProxies) == 0 {
		return ""
	}

	for _, p := range workingProxies {
		if p != currentProxy {
			return p
		}
	}

	return workingProxies[0]
}

func RemoveProxy(proxy string) {
	if proxy == "" {
		return
	}

	fmt.Printf("❌ Removing non-working proxy: %s\n", proxy)
	proxyMutex.Lock()
	defer proxyMutex.Unlock()

	for i, p := range workingProxies {
		if p == proxy {
			workingProxies = append(workingProxies[:i], workingProxies[i+1:]...)
			break
		}
	}
}

func getProxyList() ([]string, error) {
	fmt.Println("📡 Fetching proxy list from external service...")
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(ProxyListURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy list: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read proxy list: %v", err)
	}

	proxies := strings.Fields(string(body))
	if len(proxies) == 0 {
		return nil, fmt.Errorf("empty proxy list received")
	}

	rand.Shuffle(len(proxies), func(i, j int) {
		proxies[i], proxies[j] = proxies[j], proxies[i]
	})

	fmt.Printf("📋 Retrieved %d potential proxies\n", len(proxies))
	return proxies, nil
}

func checkProxy(proxy string) bool {
	if proxy == "" {
		return false
	}

	proxyURL, err := url.Parse("http://" + proxy)
	if err != nil {
		return false
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: ProxyTestTimeout,
	}

	testReq := map[string]interface{}{
		"model": TestModelID,
		"messages": []map[string]string{
			{"role": "user", "content": "test"},
		},
		"max_tokens": 1,
	}

	reqBody, _ := json.Marshal(testReq)

	req, err := http.NewRequest("POST", DeepInfraBaseURL+ChatEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Deepinfra-Source", "web-page")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized
}
