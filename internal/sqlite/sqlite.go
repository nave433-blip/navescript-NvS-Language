package sqlite

import (
	"fmt"
	"strings"
	"sync"
)

// Minimal in-memory table store (not full SQL) to avoid heavy deps.
// Supports: CREATE TABLE name (cols...), INSERT INTO name VALUES (...), SELECT * FROM name

type table struct {
	cols []string
	rows [][]string
}

var (
	mu   sync.Mutex
	dbs  = map[string]map[string]*table{}
	next = 1
)

func Open(path string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	id := fmt.Sprintf("db%d", next)
	next++
	dbs[id] = map[string]*table{}
	return id, nil
}

func Exec(id, query string) (int64, error) {
	mu.Lock()
	defer mu.Unlock()
	db, ok := dbs[id]
	if !ok {
		return 0, fmt.Errorf("unknown db handle: %s", id)
	}
	q := strings.TrimSpace(query)
	uq := strings.ToUpper(q)

	if strings.HasPrefix(uq, "CREATE TABLE") {
		// CREATE TABLE users (id, name)
		rest := strings.TrimSpace(q[len("CREATE TABLE"):])
		parts := strings.SplitN(rest, "(", 2)
		if len(parts) != 2 {
			return 0, fmt.Errorf("bad CREATE TABLE")
		}
		name := strings.TrimSpace(parts[0])
		colsPart := strings.TrimSuffix(strings.TrimSpace(parts[1]), ")")
		var cols []string
		for _, c := range strings.Split(colsPart, ",") {
			c = strings.TrimSpace(c)
			// take first token as column name
			fields := strings.Fields(c)
			if len(fields) > 0 {
				cols = append(cols, fields[0])
			}
		}
		db[name] = &table{cols: cols}
		return 0, nil
	}

	if strings.HasPrefix(uq, "INSERT INTO") {
		// INSERT INTO users VALUES (1, 'Nave')
		rest := strings.TrimSpace(q[len("INSERT INTO"):])
		// name VALUES (...)
		vi := strings.Index(strings.ToUpper(rest), "VALUES")
		if vi < 0 {
			return 0, fmt.Errorf("bad INSERT")
		}
		name := strings.TrimSpace(rest[:vi])
		valsPart := strings.TrimSpace(rest[vi+6:])
		valsPart = strings.TrimPrefix(valsPart, "(")
		valsPart = strings.TrimSuffix(valsPart, ")")
		t, ok := db[name]
		if !ok {
			return 0, fmt.Errorf("no such table: %s", name)
		}
		var row []string
		for _, v := range splitCSV(valsPart) {
			v = strings.TrimSpace(v)
			v = strings.Trim(v, "'\"")
			row = append(row, v)
		}
		t.rows = append(t.rows, row)
		return 1, nil
	}

	return 0, fmt.Errorf("unsupported exec: %s", query)
}

func Query(id, query string) ([][]string, []string, error) {
	mu.Lock()
	defer mu.Unlock()
	db, ok := dbs[id]
	if !ok {
		return nil, nil, fmt.Errorf("unknown db handle: %s", id)
	}
	q := strings.TrimSpace(query)
	uq := strings.ToUpper(q)
	if !strings.HasPrefix(uq, "SELECT") {
		return nil, nil, fmt.Errorf("only SELECT supported")
	}
	// SELECT * FROM name  or SELECT col FROM name
	fromIdx := strings.Index(uq, "FROM")
	if fromIdx < 0 {
		return nil, nil, fmt.Errorf("bad SELECT")
	}
	name := strings.TrimSpace(q[fromIdx+4:])
	// strip ORDER BY etc
	if i := strings.Index(strings.ToUpper(name), "ORDER"); i >= 0 {
		name = strings.TrimSpace(name[:i])
	}
	t, ok := db[name]
	if !ok {
		return nil, nil, fmt.Errorf("no such table: %s", name)
	}
	return t.rows, t.cols, nil
}

func Close(id string) error {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := dbs[id]; !ok {
		return fmt.Errorf("unknown db handle: %s", id)
	}
	delete(dbs, id)
	return nil
}

func splitCSV(s string) []string {
	var out []string
	var cur strings.Builder
	inQ := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' || c == '"' {
			inQ = !inQ
			cur.WriteByte(c)
			continue
		}
		if c == ',' && !inQ {
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
