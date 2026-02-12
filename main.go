package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"nhooyr.io/websocket"
)

// Page d'accueil
func handleHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
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

	log.Println("Client connecté via WebSocket")

	// 3. CONTEXT : Nécessaire pour Read/Write
	ctx := context.Background()

	// 4. BOUCLE INFINIE : Lire les messages en continu
	for {
		// READ : Attendre et lire un message du client
		_, message, err := conn.Read(ctx)

		// Si erreur (déconnexion, etc.) => sortir de la boucle
		if err != nil {
			log.Printf("Erreur lecture: %v", err)
			break
		}

		// Afficher le message reçu
		log.Printf("Message reçu: %s", string(message))

		// WRITE : Renvoyer le même message (echo)
		err = conn.Write(ctx, websocket.MessageText, message)
		if err != nil {
			log.Printf("Erreur écriture: %v", err)
			break
		}
	}

	log.Println("Client déconnecté")
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
