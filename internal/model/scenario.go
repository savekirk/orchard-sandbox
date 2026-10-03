package model

import (
	"hash/fnv"
	"strings"
	"time"
)

// Scenario decides how a payment settles. It is chosen from the customer
// number (or card number) so tests can trigger outcomes without extra setup.
type Scenario struct {
	Key         string `json:"key"`
	Suffix      string `json:"suffix"`
	Description string `json:"description"`
	Success     bool   `json:"-"`
	Message     string `json:"-"`
	NeverSettle bool   `json:"-"`
	NoCallback  bool   `json:"-"`
}

// Scenarios selected by the last digits of a customer number.
var (
	ScenarioSuccess           = Scenario{Key: "success", Description: "Succeeds after the settle delay", Success: true, Message: "SUCCESS"}
	ScenarioDeclined          = Scenario{Key: "declined", Suffix: "000001", Description: "Fails: customer declined the prompt", Message: "FAILED: Customer declined the transaction"}
	ScenarioInsufficientFunds = Scenario{Key: "insufficient_funds", Suffix: "000002", Description: "Fails: insufficient funds in customer wallet", Message: "FAILED: Insufficient funds in customer wallet"}
	ScenarioInvalidAccount    = Scenario{Key: "invalid_account", Suffix: "000003", Description: "Fails: invalid customer account (AII returns 067)", Message: "FAILED: Invalid customer account"}
	ScenarioNeverSettles      = Scenario{Key: "never_settles", Suffix: "000004", Description: "Stays pending until resolved manually", NeverSettle: true}
	ScenarioCallbackLost      = Scenario{Key: "callback_lost", Suffix: "000005", Description: "Succeeds, but no callback is sent", Success: true, Message: "SUCCESS", NoCallback: true}
)

// Scenarios lists the scenarios triggered by magic customer numbers.
var Scenarios = []Scenario{ScenarioDeclined, ScenarioInsufficientFunds, ScenarioInvalidAccount, ScenarioNeverSettles, ScenarioCallbackLost}

// ScenarioFor picks the scenario for a customer or card number.
func ScenarioFor(number string) Scenario {
	digits := onlyDigits(number)
	for _, s := range Scenarios {
		if strings.HasSuffix(digits, s.Suffix) {
			return s
		}
	}
	return ScenarioSuccess
}

// ScenarioByKey looks a scenario up by its key.
func ScenarioByKey(key string) Scenario {
	for _, s := range Scenarios {
		if s.Key == key {
			return s
		}
	}
	return ScenarioSuccess
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// CardScenario describes the identity returned for unregistered Ghana Cards,
// chosen by the card's last digit.
type CardScenario struct {
	Digit       string `json:"digit"`
	Description string `json:"description"`
}

// CardScenarios documents the last-digit rules applied by SimulatedCard.
var CardScenarios = []CardScenario{
	{"9", "Not verified (face mismatch)"},
	{"2", "Card expired"},
	{"3", "Card not yet valid"},
	{"4", "Different name on card"},
	{"5", "Validity dates missing"},
	{"other", "Verified, valid card"},
}

// SimulatedCard returns identity data for a Ghana Card with no registered profile.
func SimulatedCard(idNum string, now time.Time) VerifyIDData {
	d := VerifyIDData{Name: "Ama Mensah", Gender: "F", Verified: "true", CardValidStart: "2021-03-15", CardValidEnd: now.AddDate(5, 0, 0).Format("2006-01-02")}
	switch idNum[len(idNum)-1] {
	case '9':
		d.Verified = "false"
	case '2':
		d.CardValidStart, d.CardValidEnd = now.AddDate(-10, -1, 0).Format("2006-01-02"), now.AddDate(0, -1, 0).Format("2006-01-02")
	case '3':
		d.CardValidStart, d.CardValidEnd = now.AddDate(0, 1, 0).Format("2006-01-02"), now.AddDate(10, 1, 0).Format("2006-01-02")
	case '4':
		d.Name, d.Gender = "Kojo Owusu", "M"
	case '5':
		d.CardValidStart, d.CardValidEnd = "", ""
	}
	return d
}

var accountNames = []string{"Ama Mensah", "Kofi Boateng", "Akosua Owusu", "Kwame Asante", "Efua Darko", "Yaw Adjei", "Abena Osei", "Kojo Appiah"}

// SimulatedAccountName returns a stable name for an account with no registered profile.
func SimulatedAccountName(accountNumber string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(accountNumber))
	return accountNames[h.Sum32()%uint32(len(accountNames))]
}
