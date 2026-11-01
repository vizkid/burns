package store

import "time"

// Pane is one model's tokens in one pane that has an open binding. Tokens leave
// out cached input: a long session rereads its context on every request, and
// counting that would rank panes by age rather than by work.
type Pane struct {
	Provider string `json:"provider"`
	Account  string `json:"account"`
	Block    string `json:"block"`
	Model    string `json:"model"`
	Tokens   int64  `json:"tokens"`
	// MainTokens is the part from the session's own thread, not its subagents.
	// The model with the most of these is the one the pane runs: a few subagents
	// on a cheap model can use far more tokens than the thread that started them.
	MainTokens int64 `json:"main_tokens"`
	// Agents counts the distinct subagents and nested sessions of the whole
	// pane, so it repeats on each model row.
	Agents int       `json:"agents"`
	Last   time.Time `json:"last"`
}

// openBindings ranks each block's open bindings, oldest first. The oldest is the
// pane's own session. A later one shares its foreground process, such as
// `claude -p` run by the outer agent's shell tool, so it is the pane's subagent.
const openBindings = `WITH o AS (SELECT provider, session, block, start,
		row_number() OVER (PARTITION BY block ORDER BY start, id) AS rank
	FROM binding WHERE "end" IS NULL) `

const paneRequests = ` FROM o JOIN request r ON r.provider = o.provider AND r.session = o.session AND r.at >= o.start
	WHERE r.model != '' AND r.model NOT LIKE '<%' `

// Panes returns each open pane's tokens per account and model, largest first,
// counting only what ran since the pane took the session. An open binding can
// outlive its pane, so a reader checks the block against Rex.
func (s *Store) Panes() ([]Pane, error) {
	agents, err := s.paneAgents()
	if err != nil {
		return nil, err
	}
	rows, err := s.rdb.Query(openBindings + `SELECT r.provider, r.account, o.block, r.model,
		sum(r.input + r.cache_write + r.output),
		sum(CASE WHEN o.rank = 1 AND r.agent = '' THEN r.input + r.cache_write + r.output ELSE 0 END), max(r.at)` +
		paneRequests + `GROUP BY r.provider, r.account, o.block, r.model ORDER BY 5 DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pane
	for rows.Next() {
		var p Pane
		var last int64
		if err := rows.Scan(&p.Provider, &p.Account, &p.Block, &p.Model, &p.Tokens, &p.MainTokens, &last); err != nil {
			return nil, err
		}
		p.Agents, p.Last = agents[p.Block], fromNanos(last)
		out = append(out, p)
	}
	return out, rows.Err()
}

// paneAgents counts each block's distinct subagents and nested sessions.
func (s *Store) paneAgents() (map[string]int, error) {
	rows, err := s.rdb.Query(openBindings + `SELECT o.block, count(DISTINCT CASE
		WHEN r.agent != '' THEN r.provider || ' ' || r.session || ' ' || r.agent
		WHEN o.rank > 1 THEN r.provider || ' ' || r.session END)` + paneRequests + `GROUP BY o.block`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var block string
		var n int
		if err := rows.Scan(&block, &n); err != nil {
			return nil, err
		}
		out[block] = n
	}
	return out, rows.Err()
}
