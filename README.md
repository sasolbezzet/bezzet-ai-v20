# 🤖 Bezzet AI v20 - Crypto Trading Assistant

Bezzet AI adalah asisten trading crypto otomatis dengan fitur lengkap untuk analisis market, sinyal trading, whale tracking, dan portfolio management.

## ✨ Fitur Utama

### 📈 Trading Signals
- Sinyal BUY/SELL/HOLD dengan analisis 3 time frame (15m, 1h, 1d)
- Rekomendasi entry dengan harga BUY, DCA, Stop Loss, Take Profit
- Top 10 crypto signals (BTC, ETH, SOL, BNB, XRP, ADA, DOGE, AVAX, DOT, LINK)

### 🐋 Whale Tracking
- Tracking 10+ institutional wallets (Satoshi, MicroStrategy, Coinbase, Binance, Lido)
- Deteksi pergerakan besar > $500k dari order book Binance
- Analisis whale & market sentiment
- Notifikasi real-time ke Telegram

### 💰 Market Data
- Harga real-time 10 crypto top
- Detail harga dengan volume, high, low
- Top movers 24h

### 🎯 Smart Entry
- Rekomendasi level support & ideal entry
- Strategi entry bertahap (30%-30%-40%)

### 📁 Portfolio Management
- Tracking aset crypto
- P&L real-time
- Tambah/jual aset dengan catatan

### 🔥 Hot Tokens
- Scanner token baru potensial
- Score & market cap analysis

### 🤖 Auto Trading
- Sinyal trading otomatis
- Aktif/nonaktif dengan perintah sederhana

## 📱 Perintah Telegram

| Perintah | Fungsi |
|----------|--------|
| `/start` | Menu utama |
| `/signals` | Sinyal trading (BUY/DCA/SL/TP) |
| `/harga` | Top 10 harga crypto |
| `/harga btc` | Detail Bitcoin |
| `/whale` | Tracking institusi & pergerakan |
| `/whale_analysis` | Analisis whale & market |
| `/top` | Top movers 24h |
| `/smartentry btc` | Smart entry BTC |
| `/portfolio` | Portfolio tracking |
| `/hot` | Hot tokens |
| `/autotrade` | Status auto trading |

## 🚀 Instalasi

### Prasyarat
- Go 1.21+
- Telegram Bot Token
- DeepSeek API Key (opsional)

### Langkah Instalasi
```bash
git clone https://github.com/sasolbezzet/bezzet-ai-v20.git
cd bezzet-ai-v20
go mod download
go build -o teneo-agent main.go
./teneo-agent
```

## 📊 Data Source
- **Harga Crypto**: Binance Public API
- **Whale Tracking**: Binance Order Book + Institutional Data

## ⚠️ Disclaimer
**Bukan saran keuangan.** Selalu lakukan riset sendiri (DYOR) sebelum melakukan investasi.

## 📝 License
MIT

## 👨‍💻 Author
- Telegram: @Davabezzet
- GitHub: [sasolbezzet](https://github.com/sasolbezzet)
