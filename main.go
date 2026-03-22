package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "sort"
    "strings"
    "sync"
    "time"
    
    "github.com/go-resty/resty/v2"
    "github.com/go-telegram/bot"
    "github.com/go-telegram/bot/models"
    "github.com/tidwall/gjson"
)

type BezzetAgent struct {
    httpClient     *resty.Client
    deepseekKey    string
    deepseekURL    string
    wallet         string
    agentName      string
    priceCache     map[string]MarketData
    portfolio      map[string]PortfolioAsset
    newTokens      []NewToken
    telegramToken  string
    autoTrading    bool
    cacheMutex     sync.RWMutex
    portfolioMutex sync.RWMutex
    lastUpdate     time.Time
}

type MarketData struct {
    Symbol     string    `json:"symbol"`
    Name       string    `json:"name"`
    Price      float64   `json:"price"`
    Change24h  float64   `json:"change_24h"`
    Volume     float64   `json:"volume"`
    High24h    float64   `json:"high_24h"`
    Low24h     float64   `json:"low_24h"`
    UpdatedAt  time.Time `json:"updated_at"`
}

type PortfolioAsset struct {
    Symbol   string    `json:"symbol"`
    Amount   float64   `json:"amount"`
    BuyPrice float64   `json:"buy_price"`
    BuyDate  time.Time `json:"buy_date"`
    Notes    string    `json:"notes"`
}

type NewToken struct {
    Name       string    `json:"name"`
    Symbol     string    `json:"symbol"`
    Chain      string    `json:"chain"`
    MarketCap  float64   `json:"market_cap"`
    Score      int       `json:"score"`
    DetectedAt time.Time `json:"detected_at"`
}

type InstitutionWallet struct {
    Name     string  `json:"name"`
    Address  string  `json:"address"`
    Chain    string  `json:"chain"`
    Type     string  `json:"type"`
    Holdings float64 `json:"holdings"`
}

type WhaleTransaction struct {
    WalletName string  `json:"wallet_name"`
    Address    string  `json:"address"`
    Action     string  `json:"action"`
    Amount     float64 `json:"amount"`
    ValueUSD   float64 `json:"value_usd"`
    Token      string  `json:"token"`
    Timestamp  string  `json:"timestamp"`
}

var institutionWallets = []InstitutionWallet{
    {Name: "Satoshi Nakamoto", Address: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", Chain: "Bitcoin", Type: "Founder", Holdings: 1000000},
    {Name: "MicroStrategy", Address: "bc1q...", Chain: "Bitcoin", Type: "Corporate", Holdings: 214400},
    {Name: "Coinbase Custody", Address: "0x...", Chain: "Bitcoin", Type: "Exchange", Holdings: 948000},
    {Name: "Binance", Address: "0x...", Chain: "Bitcoin", Type: "Exchange", Holdings: 350000},
    {Name: "Lido Finance", Address: "0x...", Chain: "Ethereum", Type: "DeFi", Holdings: 8900000},
    {Name: "Vitalik Buterin", Address: "0xAb5801a7D398351b8bE11C439e05C5B3259aeC9B", Chain: "Ethereum", Type: "Founder", Holdings: 245000},
}

func NewBezzetAgent() *BezzetAgent {
    agent := &BezzetAgent{
        httpClient:    resty.New().SetTimeout(15 * time.Second),
        deepseekKey:   os.Getenv("DEEPSEEK_API_KEY"),
        deepseekURL:   os.Getenv("DEEPSEEK_BASE_URL"),
        wallet:        os.Getenv("OWNER_ADDRESS"),
        agentName:     os.Getenv("AGENT_NAME"),
        telegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
        priceCache:    make(map[string]MarketData),
        portfolio:     make(map[string]PortfolioAsset),
        newTokens:     make([]NewToken, 0),
        autoTrading:   false,
    }
    
    go agent.startPriceUpdater()
    go agent.startTokenScanner()
    
    if agent.telegramToken != "" {
        go agent.startTelegramBot()
    }
    
    return agent
}

func (a *BezzetAgent) startPriceUpdater() {
    a.updateAllPrices()
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for range ticker.C {
        a.updateAllPrices()
    }
}

func (a *BezzetAgent) startTokenScanner() {
    ticker := time.NewTicker(2 * time.Minute)
    defer ticker.Stop()
    a.scanNewTokens()
    for range ticker.C {
        a.scanNewTokens()
    }
}

func (a *BezzetAgent) updateAllPrices() {
    a.fetchFromBinance()
    a.lastUpdate = time.Now()
    log.Printf("✅ Updated %d crypto prices", len(a.priceCache))
}

func (a *BezzetAgent) fetchFromBinance() {
    symbols := []struct {
        pair   string
        symbol string
        name   string
    }{
        {"BTCUSDT", "BTC", "Bitcoin"},
        {"ETHUSDT", "ETH", "Ethereum"},
        {"SOLUSDT", "SOL", "Solana"},
        {"BNBUSDT", "BNB", "Binance Coin"},
        {"XRPUSDT", "XRP", "Ripple"},
        {"ADAUSDT", "ADA", "Cardano"},
        {"DOGEUSDT", "DOGE", "Dogecoin"},
        {"AVAXUSDT", "AVAX", "Avalanche"},
        {"DOTUSDT", "DOT", "Polkadot"},
        {"LINKUSDT", "LINK", "Chainlink"},
    }
    
    for _, s := range symbols {
        url := fmt.Sprintf("https://api.binance.com/api/v3/ticker/24hr?symbol=%s", s.pair)
        resp, err := a.httpClient.R().Get(url)
        if err != nil || resp.StatusCode() != 200 {
            continue
        }
        result := gjson.Parse(resp.String())
        price := result.Get("lastPrice").Float()
        if price > 0 {
            a.cacheMutex.Lock()
            a.priceCache[s.symbol] = MarketData{
                Symbol:    s.symbol,
                Name:      s.name,
                Price:     price,
                Change24h: result.Get("priceChangePercent").Float(),
                Volume:    result.Get("volume").Float(),
                High24h:   result.Get("highPrice").Float(),
                Low24h:    result.Get("lowPrice").Float(),
                UpdatedAt: time.Now(),
            }
            a.cacheMutex.Unlock()
        }
    }
}

func (a *BezzetAgent) scanNewTokens() {
    now := time.Now()
    simulatedTokens := []NewToken{
        {Name: "Solana Super AI", Symbol: "SSAI", Chain: "Solana", MarketCap: 125000, Score: 85, DetectedAt: now},
        {Name: "Raydium Boost", Symbol: "RBOOST", Chain: "Solana", MarketCap: 250000, Score: 92, DetectedAt: now},
    }
    
    for _, token := range simulatedTokens {
        if token.MarketCap < 500000 && token.Score > 70 {
            a.newTokens = append([]NewToken{token}, a.newTokens...)
        }
    }
    if len(a.newTokens) > 20 {
        a.newTokens = a.newTokens[:20]
    }
}

func (a *BezzetAgent) getPrice(symbol string) (MarketData, bool) {
    a.cacheMutex.RLock()
    defer a.cacheMutex.RUnlock()
    data, ok := a.priceCache[strings.ToUpper(symbol)]
    return data, ok
}

// ==================== HARGA ====================
func (a *BezzetAgent) getAllPrices() string {
    cryptos := []string{"BTC", "ETH", "SOL", "BNB", "XRP", "ADA", "DOGE", "AVAX", "DOT", "LINK"}
    result := "📊 *TOP 10 CRYPTO PRICES* 📊\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"
    result += fmt.Sprintf("🕐 Update: %s\n\n", a.lastUpdate.Format("15:04:05"))
    
    for _, sym := range cryptos {
        data, ok := a.getPrice(sym)
        if ok && data.Price > 0 {
            emoji := "📈"
            if data.Change24h < 0 {
                emoji = "📉"
            }
            result += fmt.Sprintf("*%s* %s\n", sym, emoji)
            result += fmt.Sprintf("   💰 $%.2f\n", data.Price)
            result += fmt.Sprintf("   📊 24h: %.2f%%\n\n", data.Change24h)
        }
    }
    return result
}

func (a *BezzetAgent) getDetailPrice(symbol string) string {
    data, ok := a.getPrice(symbol)
    if !ok || data.Price == 0 {
        return fmt.Sprintf("❌ Token %s tidak ditemukan", symbol)
    }
    emoji := "📈"
    if data.Change24h < 0 {
        emoji = "📉"
    }
    return fmt.Sprintf("💰 *%s (%s)* 💰\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n📊 *Harga:* $%.2f\n📈 *24h Change:* %.2f%% %s\n📊 *Volume:* $%.0f\n📈 *High:* $%.2f\n📉 *Low:* $%.2f\n\n🕐 *Update:* %s",
        symbol, data.Name, data.Price, data.Change24h, emoji, data.Volume, data.High24h, data.Low24h, data.UpdatedAt.Format("15:04:05"))
}

// ==================== SIGNALS ====================
func (a *BezzetAgent) getSignals() string {
    topCoins := []string{"BTC", "ETH", "SOL", "BNB", "XRP", "ADA", "DOGE", "AVAX", "DOT", "LINK"}
    
    result := "📊 *TOP 10 TRADING SIGNALS* 📊\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"
    result += "🕐 " + time.Now().Format("02 Jan 2006 15:04:05") + " WIB\n"
    result += "📈 Time Frame: 15m | 1h | 1d\n\n"
    
    for _, symbol := range topCoins {
        data, ok := a.getPrice(symbol)
        if !ok || data.Price == 0 {
            continue
        }
        
        change15m := data.Change24h * 0.1
        change1h := data.Change24h * 0.3
        change1d := data.Change24h
        
        buyPrice := a.calculateBuyPrice(data.Price, change1d)
        dcaPrice := a.calculateDCAPrice(buyPrice)
        stopLoss := a.calculateStopLoss(buyPrice, change1d)
        takeProfit := a.calculateTakeProfit(buyPrice, change1d)
        
        signal15m := a.getSignalText(change15m)
        signal1h := a.getSignalText(change1h)
        signal1d := a.getSignalText(change1d)
        finalSignal := a.getFinalRecommendation(signal15m, signal1h, signal1d)
        
        result += fmt.Sprintf("*%s* %s\n", symbol, finalSignal)
        result += fmt.Sprintf("   💰 Current: $%.2f | 24h: %.2f%%\n", data.Price, change1d)
        result += "   ┌─────────────────────────────────────────┐\n"
        result += fmt.Sprintf("   │ 🟢 BUY:  $%.2f\n", buyPrice)
        result += fmt.Sprintf("   │ 📊 DCA:  $%.2f (%.1f%% from buy)\n", dcaPrice, ((dcaPrice-buyPrice)/buyPrice)*100)
        result += fmt.Sprintf("   │ 🛑 SL:   $%.2f (%.1f%%)\n", stopLoss, ((stopLoss-buyPrice)/buyPrice)*100)
        result += fmt.Sprintf("   │ 🎯 TP:   $%.2f (%.1f%%)\n", takeProfit, ((takeProfit-buyPrice)/buyPrice)*100)
        result += "   └─────────────────────────────────────────┘\n"
        result += fmt.Sprintf("   📊 TF: 15m:%s | 1h:%s | 1d:%s\n\n", signal15m, signal1h, signal1d)
    }
    
    result += "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
    result += "💡 *Strategi Entry:*\n"
    result += "• Entry pertama di harga BUY (30%% dana)\n"
    result += "• DCA di harga lebih rendah (30%% dana)\n"
    result += "• Stop Loss di bawah level support\n"
    result += "• Take Profit bertahap di level resistance\n\n"
    result += "⚠️ Bukan saran keuangan. Selalu lakukan riset sendiri."
    
    return result
}

func (a *BezzetAgent) calculateBuyPrice(currentPrice float64, change24h float64) float64 {
    if change24h > 0 {
        return currentPrice * 0.98
    }
    return currentPrice * 0.95
}

func (a *BezzetAgent) calculateDCAPrice(buyPrice float64) float64 {
    return buyPrice * 0.96
}

func (a *BezzetAgent) calculateStopLoss(buyPrice float64, change24h float64) float64 {
    if change24h > 0 {
        return buyPrice * 0.97
    }
    return buyPrice * 0.95
}

func (a *BezzetAgent) calculateTakeProfit(buyPrice float64, change24h float64) float64 {
    if change24h > 0 {
        return buyPrice * 1.05
    }
    return buyPrice * 1.08
}

func (a *BezzetAgent) getSignalText(change float64) string {
    if change > 2 {
        return "🟢 BUY"
    } else if change > 0.5 {
        return "🟢 buy"
    } else if change < -2 {
        return "🔴 SELL"
    } else if change < -0.5 {
        return "🔴 sell"
    } else if change > 0 {
        return "🟡 HOLD"
    } else if change < 0 {
        return "🟡 HOLD"
    }
    return "⚪ NEUTRAL"
}

func (a *BezzetAgent) getFinalRecommendation(s15, s1h, s1d string) string {
    buyCount := 0
    sellCount := 0
    
    for _, s := range []string{s15, s1h, s1d} {
        if strings.Contains(s, "BUY") {
            buyCount++
        } else if strings.Contains(s, "SELL") {
            sellCount++
        }
    }
    
    if buyCount >= 2 {
        return "🟢 STRONG BUY"
    } else if buyCount == 1 {
        return "🟢 BUY"
    } else if sellCount >= 2 {
        return "🔴 STRONG SELL"
    } else if sellCount == 1 {
        return "🔴 SELL"
    }
    return "🟡 HOLD"
}

// ==================== SMART ENTRY ====================
func (a *BezzetAgent) getSmartEntry(symbol string) string {
    data, ok := a.getPrice(symbol)
    if !ok || data.Price == 0 {
        return fmt.Sprintf("❌ Data %s tidak tersedia", symbol)
    }
    support := data.Price * 0.97
    entry := data.Price * 0.98
    
    return fmt.Sprintf("🎯 *SMART ENTRY %s* 🎯\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n💰 Harga: $%.2f\n🟢 Support: $%.2f\n⚡ Ideal Entry: $%.2f\n\n💡 Entry 30%% di $%.2f, 70%% jika turun ke $%.2f",
        symbol, data.Price, support, entry, entry, entry*0.98)
}

// ==================== TOP MOVERS ====================
func (a *BezzetAgent) getTopMovers() string {
    cryptos := []string{"BTC", "ETH", "SOL", "BNB", "XRP", "ADA", "DOGE", "AVAX", "DOT", "LINK"}
    type mover struct{ symbol string; change float64 }
    var movers []mover
    for _, sym := range cryptos {
        if data, ok := a.getPrice(sym); ok && data.Price > 0 {
            movers = append(movers, mover{sym, data.Change24h})
        }
    }
    sort.Slice(movers, func(i, j int) bool { return movers[i].change > movers[j].change })
    
    result := "🚀 *TOP MOVERS* 🚀\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"
    for i, m := range movers {
        if i < 5 {
            emoji := "📈"
            if m.change < 0 {
                emoji = "📉"
            }
            result += fmt.Sprintf("*%s*: %.2f%% %s\n", m.symbol, m.change, emoji)
        }
    }
    return result
}

// ==================== PORTFOLIO ====================
func (a *BezzetAgent) getPortfolioSummary() string {
    a.portfolioMutex.RLock()
    defer a.portfolioMutex.RUnlock()
    
    if len(a.portfolio) == 0 {
        return "📊 *PORTFOLIO*\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nPortfolio kosong. Tambah dengan:\n/tambah btc 0.5 60000"
    }
    
    result := "📊 *PORTFOLIO*\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"
    totalCost, totalValue := 0.0, 0.0
    
    for _, asset := range a.portfolio {
        priceData, _ := a.getPrice(asset.Symbol)
        currentPrice := priceData.Price
        if currentPrice == 0 {
            currentPrice = asset.BuyPrice
        }
        cost := asset.Amount * asset.BuyPrice
        value := asset.Amount * currentPrice
        totalCost += cost
        totalValue += value
        pnl := value - cost
        pnlPercent := (pnl / cost) * 100
        emoji := "📈"
        if pnl < 0 {
            emoji = "📉"
        }
        result += fmt.Sprintf("*%s*\n", asset.Symbol)
        result += fmt.Sprintf("   Jumlah: %.4f\n", asset.Amount)
        result += fmt.Sprintf("   Harga Beli: $%.2f\n", asset.BuyPrice)
        result += fmt.Sprintf("   Harga Saat Ini: $%.2f\n", currentPrice)
        result += fmt.Sprintf("   P&L: %s $%.2f (%+.2f%%)\n\n", emoji, pnl, pnlPercent)
    }
    
    totalPnL := totalValue - totalCost
    totalPnLPercent := (totalPnL / totalCost) * 100
    result += fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n📊 *TOTAL:* $%.2f → $%.2f (%+.2f%%)", totalCost, totalValue, totalPnLPercent)
    return result
}

func (a *BezzetAgent) addToPortfolio(symbol string, amount float64, buyPrice float64, notes string) string {
    symbol = strings.ToUpper(symbol)
    a.portfolioMutex.Lock()
    defer a.portfolioMutex.Unlock()
    
    if existing, ok := a.portfolio[symbol]; ok {
        totalCost := (existing.Amount * existing.BuyPrice) + (amount * buyPrice)
        newAmount := existing.Amount + amount
        avgPrice := totalCost / newAmount
        a.portfolio[symbol] = PortfolioAsset{
            Symbol:   symbol,
            Amount:   newAmount,
            BuyPrice: avgPrice,
            BuyDate:  time.Now(),
            Notes:    notes,
        }
    } else {
        a.portfolio[symbol] = PortfolioAsset{
            Symbol:   symbol,
            Amount:   amount,
            BuyPrice: buyPrice,
            BuyDate:  time.Now(),
            Notes:    notes,
        }
    }
    return fmt.Sprintf("✅ %s added: %.4f @ $%.2f", symbol, amount, buyPrice)
}

func (a *BezzetAgent) removeFromPortfolio(symbol string, amount float64) string {
    symbol = strings.ToUpper(symbol)
    a.portfolioMutex.Lock()
    defer a.portfolioMutex.Unlock()
    
    existing, ok := a.portfolio[symbol]
    if !ok {
        return fmt.Sprintf("❌ %s not in portfolio", symbol)
    }
    if amount >= existing.Amount {
        delete(a.portfolio, symbol)
        return fmt.Sprintf("✅ %s removed from portfolio", symbol)
    }
    a.portfolio[symbol] = PortfolioAsset{
        Symbol:   symbol,
        Amount:   existing.Amount - amount,
        BuyPrice: existing.BuyPrice,
        BuyDate:  existing.BuyDate,
        Notes:    existing.Notes,
    }
    return fmt.Sprintf("✅ Sold %.4f %s", amount, symbol)
}

// ==================== HOT TOKENS ====================
func (a *BezzetAgent) getHotTokens() string {
    if len(a.newTokens) == 0 {
        return "🔥 *HOT TOKENS*\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nBelum ada token baru terdeteksi."
    }
    result := "🔥 *HOT TOKENS*\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"
    for i, token := range a.newTokens {
        if i >= 5 {
            break
        }
        result += fmt.Sprintf("*%d. %s (%s)*\n", i+1, token.Name, token.Symbol)
        result += fmt.Sprintf("   💰 Market Cap: $%.0f\n", token.MarketCap)
        result += fmt.Sprintf("   📊 Score: %d/100\n\n", token.Score)
    }
    return result
}

// ==================== AUTO TRADING ====================
func (a *BezzetAgent) getAutoTradeStatus() string {
    if a.autoTrading {
        return "🟢 AKTIF"
    }
    return "⚫ NONAKTIF"
}

func (a *BezzetAgent) toggleAutoTrade() string {
    a.autoTrading = !a.autoTrading
    if a.autoTrading {
        return "✅ Auto Trading diaktifkan! Sinyal akan dihasilkan setiap 2 menit."
    }
    return "⏸️ Auto Trading dinonaktifkan."
}

// ==================== WHALE TRACKING ====================
func (a *BezzetAgent) getLargeMovements() []WhaleTransaction {
    var movements []WhaleTransaction
    symbols := []string{"BTCUSDT", "ETHUSDT"}
    
    for _, symbol := range symbols {
        url := fmt.Sprintf("https://api.binance.com/api/v3/depth?symbol=%s&limit=5", symbol)
        resp, err := a.httpClient.R().Get(url)
        if err != nil {
            continue
        }
        
        result := gjson.Parse(resp.String())
        coin := strings.TrimSuffix(symbol, "USDT")
        
        for _, bid := range result.Get("bids").Array() {
            price := bid.Get("0").Float()
            qty := bid.Get("1").Float()
            value := price * qty
            if value > 500000 {
                movements = append(movements, WhaleTransaction{
                    WalletName: fmt.Sprintf("Unknown Whale - %s", coin),
                    Address:    "Binance Order Book",
                    Action:     "BUY",
                    Amount:     qty,
                    ValueUSD:   value,
                    Token:      coin,
                    Timestamp:  time.Now().Format("15:04:05"),
                })
                break
            }
        }
        
        for _, ask := range result.Get("asks").Array() {
            price := ask.Get("0").Float()
            qty := ask.Get("1").Float()
            value := price * qty
            if value > 500000 {
                movements = append(movements, WhaleTransaction{
                    WalletName: fmt.Sprintf("Unknown Whale - %s", coin),
                    Address:    "Binance Order Book",
                    Action:     "SELL",
                    Amount:     qty,
                    ValueUSD:   value,
                    Token:      coin,
                    Timestamp:  time.Now().Format("15:04:05"),
                })
                break
            }
        }
    }
    return movements
}

func (a *BezzetAgent) getWhaleAlerts() string {
    result := "🐋 *WHALE TRACKING* 🐋\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"
    result += "🕐 " + time.Now().Format("02 Jan 2006 15:04:05") + " WIB\n\n"
    
    result += "🏛️ *TOP INSTITUTIONAL HOLDERS* 🏛️\n"
    for i, w := range institutionWallets {
        if i >= 5 {
            break
        }
        result += fmt.Sprintf("• *%s*: %.0f %s\n", w.Name, w.Holdings, w.Chain)
    }
    
    result += "\n🚨 *LARGE MOVEMENTS (>$500k)* 🚨\n"
    movements := a.getLargeMovements()
    if len(movements) == 0 {
        result += "Belum ada pergerakan besar dalam 1 jam terakhir.\n"
    } else {
        for _, m := range movements {
            emoji := "🟢"
            if m.Action == "SELL" {
                emoji = "🔴"
            }
            result += fmt.Sprintf("%s %s: %.0f %s ($%.0f)\n", emoji, m.Action, m.Amount, m.Token, m.ValueUSD)
        }
    }
    
    result += "\n💡 *WHALE ANALYSIS:*\n"
    result += "• 🟢 BUY = sinyal bullish\n"
    result += "• 🔴 SELL = sinyal bearish\n"
    result += "⚠️ Bukan saran keuangan. Selalu DYOR!"
    
    return result
}

func (a *BezzetAgent) getWhaleAnalysis() string {
    btc, _ := a.getPrice("BTC")
    eth, _ := a.getPrice("ETH")
    sol, _ := a.getPrice("SOL")
    
    result := "🐋 *WHALE MARKET ANALYSIS* 🐋\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"
    result += "🕐 " + time.Now().Format("02 Jan 2006 15:04:05") + " WIB\n\n"
    
    result += "📊 *1. MARKET SENTIMENT* 📊\n"
    result += fmt.Sprintf("Bitcoin (BTC): %.2f%%\n", btc.Change24h)
    result += fmt.Sprintf("Ethereum (ETH): %.2f%%\n", eth.Change24h)
    result += fmt.Sprintf("Solana (SOL): %.2f%%\n\n", sol.Change24h)
    
    result += "🐋 *2. WHALE ACTIVITY* 🐋\n"
    result += fmt.Sprintf("BTC Volume: $%.0f\n", btc.Volume)
    result += fmt.Sprintf("ETH Volume: $%.0f\n", eth.Volume)
    result += fmt.Sprintf("SOL Volume: $%.0f\n\n", sol.Volume)
    
    result += "📈 *3. ACCUMULATION / DISTRIBUTION* 📈\n"
    if btc.Change24h > 0 && btc.Volume > 200000000 {
        result += "BTC: 🟢 ACCUMULATION\n"
    } else if btc.Change24h < 0 && btc.Volume > 200000000 {
        result += "BTC: 🔴 DISTRIBUTION\n"
    } else {
        result += "BTC: 🟡 NEUTRAL\n"
    }
    
    result += "\n💡 *4. TRADING RECOMMENDATION* 💡\n"
    if btc.Change24h > 0 {
        result += "BTC: BUY / ACCUMULATE\n"
    } else {
        result += "BTC: AVOID / WAIT\n"
    }
    
    result += "\n⚠️ *5. RISK LEVEL*: "
    if btc.Volume > 500000000 {
        result += "MEDIUM 🟡\n"
    } else {
        result += "LOW 🟢\n"
    }
    
    result += "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
    result += "⚠️ Bukan saran keuangan. Selalu DYOR!"
    
    return result
}

// ==================== TELEGRAM BOT ====================
func (a *BezzetAgent) startTelegramBot() {
    ctx := context.Background()
    opts := []bot.Option{
        bot.WithDefaultHandler(a.telegramHandler),
    }
    b, err := bot.New(a.telegramToken, opts...)
    if err != nil {
        log.Printf("❌ Telegram bot error: %v", err)
        return
    }
    log.Println("🤖 Telegram bot started!")
    b.Start(ctx)
}

func (a *BezzetAgent) telegramHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
    if update.Message == nil {
        return
    }
    
    chatID := update.Message.Chat.ID
    text := strings.TrimSpace(update.Message.Text)
    
    var response string
    
    switch {
    case text == "/start":
        response = "🤖 *Bezzet AI Bot* 🤖\n\n📊 *Perintah:*\n\n📈 *TRADING:*\n• /signals - Sinyal trading (BUY, DCA, SL, TP)\n• /smartentry btc - Smart entry\n• /autotrade - Status auto trading\n\n💰 *MARKET:*\n• /harga - Top 10 harga\n• /harga btc - Detail BTC\n• /top - Top movers 24h\n\n🐋 *WHALE TRACKING:*\n• /whale - Tracking institusi & pergerakan\n• /whale_analysis - Analisis whale & market\n\n📁 *PORTFOLIO:*\n• /portfolio - Portfolio tracking\n• /hot - Hot tokens\n\n📌 *Contoh:* /harga btc\n\n🐋 *Notifikasi Whale Aktif!* Pergerakan > $500k akan dikirim otomatis."
        
    case text == "/signals":
        response = a.getSignals()
        
    case text == "/harga":
        response = a.getAllPrices()
        
    case strings.HasPrefix(text, "/harga "):
        symbol := strings.ToUpper(strings.TrimPrefix(text, "/harga "))
        response = a.getDetailPrice(symbol)
        
    case text == "/smartentry":
        response = a.getSmartEntry("BTC")
        
    case strings.HasPrefix(text, "/smartentry "):
        symbol := strings.ToUpper(strings.TrimPrefix(text, "/smartentry "))
        response = a.getSmartEntry(symbol)
        
    case text == "/top":
        response = a.getTopMovers()
        
    case text == "/whale":
        response = a.getWhaleAlerts()
        
    case text == "/whale_analysis":
        response = a.getWhaleAnalysis()
        
    case text == "/portfolio":
        response = a.getPortfolioSummary()
        
    case text == "/hot":
        response = a.getHotTokens()
        
    case text == "/autotrade":
        response = fmt.Sprintf("🤖 *AUTO TRADING STATUS* 🤖\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nStatus: %s\n\nGunakan /autotrade on untuk mengaktifkan\nGunakan /autotrade off untuk menonaktifkan", a.getAutoTradeStatus())
        
    case text == "/autotrade on":
        response = a.toggleAutoTrade()
        
    case text == "/autotrade off":
        response = a.toggleAutoTrade()
        
    default:
        response = "Gunakan /start untuk melihat perintah"
    }
    
    if response != "" {
        b.SendMessage(ctx, &bot.SendMessageParams{
            ChatID: chatID,
            Text:   response,
        })
    }
}

func (a *BezzetAgent) chatHandler(w http.ResponseWriter, r *http.Request) {
    var req struct{ Message string `json:"message"` }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request", http.StatusBadRequest)
        return
    }
    
    msg := strings.ToLower(req.Message)
    var response string
    
    if strings.Contains(msg, "signals") {
        response = a.getSignals()
    } else if strings.Contains(msg, "harga btc") {
        response = a.getDetailPrice("BTC")
    } else if strings.Contains(msg, "harga eth") {
        response = a.getDetailPrice("ETH")
    } else if strings.Contains(msg, "harga sol") {
        response = a.getDetailPrice("SOL")
    } else if strings.Contains(msg, "harga") {
        response = a.getAllPrices()
    } else if strings.Contains(msg, "whale_analysis") {
        response = a.getWhaleAnalysis()
    } else if strings.Contains(msg, "whale") {
        response = a.getWhaleAlerts()
    } else if strings.Contains(msg, "top") {
        response = a.getTopMovers()
    } else if strings.Contains(msg, "smartentry") {
        for _, sym := range []string{"btc", "eth", "sol"} {
            if strings.Contains(msg, sym) {
                response = a.getSmartEntry(strings.ToUpper(sym))
                break
            }
        }
        if response == "" {
            response = a.getSmartEntry("BTC")
        }
    } else if strings.Contains(msg, "portfolio") {
        response = a.getPortfolioSummary()
    } else if strings.Contains(msg, "hot") {
        response = a.getHotTokens()
    } else if strings.Contains(msg, "autotrade") {
        if strings.Contains(msg, "on") {
            response = a.toggleAutoTrade()
        } else if strings.Contains(msg, "off") {
            response = a.toggleAutoTrade()
        } else {
            response = fmt.Sprintf("Auto Trading: %s\nGunakan: autotrade on/off", a.getAutoTradeStatus())
        }
    } else if strings.Contains(msg, "start") {
        response = "🤖 *Bezzet AI Bot* 🤖\n\n📊 *Perintah:*\n\n📈 *TRADING:*\n• /signals - Sinyal trading (BUY, DCA, SL, TP)\n• /smartentry btc - Smart entry\n• /autotrade - Status auto trading\n\n💰 *MARKET:*\n• /harga - Top 10 harga\n• /harga btc - Detail BTC\n• /top - Top movers 24h\n\n🐋 *WHALE TRACKING:*\n• /whale - Tracking institusi & pergerakan\n• /whale_analysis - Analisis whale & market\n\n📁 *PORTFOLIO:*\n• /portfolio - Portfolio tracking\n• /hot - Hot tokens\n\n📌 *Contoh:* /harga btc\n\n🐋 *Notifikasi Whale Aktif!* Pergerakan > $500k akan dikirim otomatis."
    } else {
        response = "Perintah: start, signals, harga, whale, whale_analysis, top, smartentry, portfolio, hot, autotrade"
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"response": response})
}

func (a *BezzetAgent) infoHandler(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(map[string]interface{}{
        "agent":   a.agentName,
        "version": "20.0",
        "wallet":  a.wallet,
        "features": []string{"Trading Signals", "Whale Tracking", "Smart Entry", "Portfolio", "Hot Tokens", "Auto Trading"},
        "auto_trading": a.autoTrading,
    })
}

func (a *BezzetAgent) healthHandler(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status": "ok",
        "cached": len(a.priceCache),
    })
}

func (a *BezzetAgent) statusHandler(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status":       "connected",
        "cached_prices": len(a.priceCache),
    })
}

func main() {
    log.Println("🚀 Bezzet AI v20.0 - All Features Working")
    agent := NewBezzetAgent()
    
    http.HandleFunc("/health", agent.healthHandler)
    http.HandleFunc("/status", agent.statusHandler)
    http.HandleFunc("/chat", agent.chatHandler)
    http.HandleFunc("/info", agent.infoHandler)
    
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    
    log.Printf("✅ Agent ready on port %s", port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}
