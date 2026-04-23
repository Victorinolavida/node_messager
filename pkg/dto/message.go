package dto

type Message struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	FromNode  string `json:"from_node"`
	ToNode    string `json:"to_node"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}
