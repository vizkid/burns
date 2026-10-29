package store

import (
	"database/sql"
	"errors"
)

// OTel events name no account (docs/SPEC.md, "Claude tokens"). A row without
// one takes its session's account, which comes from the transcript's home.

// attribution remembers, within one transaction, the sessions already looked
// up or filled, so a batch touches each session once.
type attribution struct {
	tx    *sql.Tx
	known map[[2]string]string
}

func newAttribution(tx *sql.Tx) *attribution {
	return &attribution{tx: tx, known: map[[2]string]string{}}
}

// fill sets *account from the session when it is empty. When it is set, the
// session's unattributed rows take it.
func (a *attribution) fill(provider, session string, account *string) error {
	k := [2]string{provider, session}
	if *account == "" {
		acct, seen := a.known[k]
		if !seen {
			err := a.tx.QueryRow(`SELECT account FROM request WHERE provider = ? AND session = ? AND account != '' LIMIT 1`,
				provider, session).Scan(&acct)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			a.known[k] = acct
		}
		*account = acct
		return nil
	}
	if a.known[k] == *account {
		return nil
	}
	a.known[k] = *account
	for _, table := range []string{"request", "compaction"} {
		if _, err := a.tx.Exec(`UPDATE `+table+` SET account = ? WHERE provider = ? AND session = ? AND account = ''`,
			*account, provider, session); err != nil {
			return err
		}
	}
	return nil
}
