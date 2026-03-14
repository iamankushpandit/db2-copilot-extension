package database

type Schema struct {
	Tables []Table
}

type Table struct {
	Schema      string
	Name        string
	Columns     []Column
	Constraints []Constraint
	Comment     string
}

type Column struct {
	Name    string
	Type    string
	Comment string
}

type Constraint struct {
	Name       string
	Type       string // PK, FK
	Columns    []string
	RefTable   string
	RefColumns []string
}
