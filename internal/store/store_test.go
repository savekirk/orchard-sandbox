package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/savekirk/orchard-sandbox/internal/model"
)

func TestFileDatabaseSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sandbox.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureDefaultRun(NewRun{ClientKey: "ck", SecretKey: "sk", ServiceID: "1", Balances: map[string]int64{model.AccountPayout: 500}}); err != nil {
		t.Fatal(err)
	}
	txn := &model.Transaction{RunID: DefaultRunID, Exttrid: "X1", TransType: "MTC", Channel: model.ChannelAPI, AmountPesewas: 200, Scenario: "success"}
	if err := st.CreateTransaction(txn); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.GetTransaction(DefaultRunID, "X1"); err != nil {
		t.Fatalf("transaction lost after restart: %v", err)
	}
	if b, _ := st.Balances(DefaultRunID); b[model.AccountPayout] != 300 {
		t.Fatalf("payout reservation lost after restart: %v", b)
	}
}

func TestDeleteRunRemovesItsData(t *testing.T) {
	st, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.CreateRun(NewRun{RunID: "temp"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTransaction(&model.Transaction{RunID: "temp", Exttrid: "T1", TransType: "CTM", AmountPesewas: 100}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteRun("temp"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetTransaction("temp", "T1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("transactions should be deleted with their run, got %v", err)
	}
	if err := st.Reserve("temp", RefExttrid, "T1"); err == nil {
		t.Fatal("reserving in a deleted run should fail on the foreign key")
	}
}
