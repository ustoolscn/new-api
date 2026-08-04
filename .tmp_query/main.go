package main
import (
  "fmt"
  "gorm.io/driver/postgres"
  "gorm.io/gorm"
  "gorm.io/gorm/logger"
)
func main() {
  db, err := gorm.Open(postgres.Open("postgres://luji:wanASP211@127.0.0.1:5000/cooper?sslmode=disable"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
  if err != nil { panic(err) }
  db.Exec("SET default_transaction_read_only = on")
  type row struct {
    PaymentProvider string
    PaymentMethod string
    Cnt int64
    MoneyMin float64
    MoneyMax float64
    MoneyAvg float64
  }
  var rows []row
  db.Raw(`SELECT COALESCE(payment_provider,'') as payment_provider, COALESCE(payment_method,'') as payment_method, count(*) as cnt, min(money) as money_min, max(money) as money_max, avg(money) as money_avg FROM subscription_orders WHERE status='success' GROUP BY 1,2 ORDER BY cnt DESC`).Scan(&rows)
  for _, r := range rows {
    fmt.Printf("provider=%q method=%q cnt=%d money[min=%.4f max=%.4f avg=%.4f]\n", r.PaymentProvider, r.PaymentMethod, r.Cnt, r.MoneyMin, r.MoneyMax, r.MoneyAvg)
  }
  var price string
  db.Raw("SELECT value FROM options WHERE key = 'Price' OR key = 'price' LIMIT 1").Scan(&price)
  fmt.Println("option Price=", price)
  // sample successful subscription top_ups
  type trow struct{ Id int; Amount int64; Money float64; Method string; Provider string; Trade string }
  var tops []trow
  db.Raw(`SELECT id, amount, money, payment_method as method, payment_provider as provider, trade_no as trade FROM top_ups WHERE amount=0 AND status='success' ORDER BY id DESC LIMIT 15`).Scan(&tops)
  for _, t := range tops {
    fmt.Printf("top id=%d amount=%d money=%.4f method=%s provider=%s trade=%s\n", t.Id, t.Amount, t.Money, t.Method, t.Provider, t.Trade)
  }
}
