package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"nhooyr.io/websocket"
)

type client struct {
	id   string
	conn *websocket.Conn
}

type inboundMsg struct {
	Type string `json:"type"` // "broadcast" | "dm"
	To   string `json:"to"`   // required for dm
	Text string `json:"text"`
}

type outboundMsg struct {
	Type string   `json:"type"`           // "welcome" | "broadcast" | "dm" | "error" | "clients"
	From string   `json:"from,omitempty"` // server sets
	To   string   `json:"to,omitempty"`
	Text string   `json:"text,omitempty"`
	ID   string   `json:"id,omitempty"`   // for welcome
	List []string `json:"list,omitempty"` // for clients
}

var (
	clientsMu sync.Mutex
	clients   = make(map[string]*client) // id -> client
)

// Page d'accueil
func handleHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func newClientID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "c-" + hex.EncodeToString(b)
}

func sendJSON(ctx context.Context, c *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Write(ctx, websocket.MessageText, b)
}

func broadcastClientsList() {
	clientsMu.Lock()
	list := make([]string, 0, len(clients))
	for id := range clients {
		list = append(list, id)
	}
	// copie conns
	conns := make([]*websocket.Conn, 0, len(clients))
	for _, cl := range clients {
		conns = append(conns, cl.conn)
	}
	clientsMu.Unlock()

	msg := outboundMsg{Type: "clients", List: list}

	for _, c := range conns {
		_ = sendJSON(context.Background(), c, msg)
	}
}

func broadcast(fromID string, text string) {
	clientsMu.Lock()
	conns := make([]*websocket.Conn, 0, len(clients))
	for _, cl := range clients {
		conns = append(conns, cl.conn)
	}
	clientsMu.Unlock()

	msg := outboundMsg{
		Type: "broadcast",
		From: fromID,
		Text: text,
	}

	for _, c := range conns {
		_ = sendJSON(context.Background(), c, msg)
	}
}

func directMessage(fromID, toID, text string) error {
	clientsMu.Lock()
	toClient := clients[toID]
	fromClient := clients[fromID]
	clientsMu.Unlock()

	if toClient == nil {
		return fmt.Errorf("client '%s' introuvable", toID)
	}
	if fromClient == nil {
		return fmt.Errorf("sender introuvable")
	}

	// message au destinataire
	err := sendJSON(context.Background(), toClient.conn, outboundMsg{
		Type: "dm",
		From: fromID,
		To:   toID,
		Text: text,
	})
	if err != nil {
		return err
	}

	// accusé local à l'expéditeur (pratique pour l'UI)
	_ = sendJSON(context.Background(), fromClient.conn, outboundMsg{
		Type: "dm",
		From: fromID,
		To:   toID,
		Text: text,
	})
	return nil
}

// Connexions Websocket
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 1. ACCEPT : Transformer la connexion HTTP en WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
	if err != nil {
		log.Printf("Erreur Accept: %v", err)
		return
	}

	// 2. DEFER CLOSE : Fermer proprement à la fin
	defer conn.Close(websocket.StatusNormalClosure, "Au revoir")

	// 3. CONTEXT : Nécessaire pour Read/Write
	ctx := context.Background()

	id := newClientID()

	clientsMu.Lock()
	clients[id] = &client{id: id, conn: conn}
	clientsMu.Unlock()

	log.Printf("Client connecté via WebSocket: %s", id)

	// message de bienvenue avec l'id
	_ = sendJSON(ctx, conn, outboundMsg{Type: "welcome", ID: id})

	// push liste des clients à tout le monde
	broadcastClientsList()

	// 4. BOUCLE INFINIE : Lire les messages en continu
	for {
		// READ : Attendre et lire un message du client
		_, message, err := conn.Read(ctx)

		// Si erreur (déconnexion, etc.) => sortir de la boucle
		if err != nil {
			log.Printf("Erreur lecture (%s): %v", id, err)
			break
		}

		// Afficher le message reçu
		log.Printf("Message reçu (%s): %s", id, string(message))

		// Si c'est du JSON => dm/broadcast, sinon on le traite comme broadcast texte brut
		var in inboundMsg
		if err := json.Unmarshal(message, &in); err != nil || in.Type == "" {
			broadcast(id, string(message))
			continue
		}

		switch in.Type {
		case "broadcast":
			broadcast(id, in.Text)
		case "dm":
			if err := directMessage(id, in.To, in.Text); err != nil {
				_ = sendJSON(ctx, conn, outboundMsg{Type: "error", Text: err.Error()})
			}
		default:
			_ = sendJSON(ctx, conn, outboundMsg{Type: "error", Text: "type inconnu"})
		}
	}

	clientsMu.Lock()
	delete(clients, id)
	clientsMu.Unlock()

	// push liste des clients à tout le monde
	broadcastClientsList()

	log.Printf("Client déconnecté: %s", id)
}

func main() {
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/ws", handleWebSocket)

	fmt.Println("Serveur démarré sur http://localhost:8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("Erreur serveur:", err)
	}
}
