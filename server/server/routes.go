package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/shi-gg/linkdave/server/protocol"
)

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /sessions/{session_id}/players/{guild_id}/play", s.withSession(s.routePlay))
	mux.HandleFunc("POST /sessions/{session_id}/players/{guild_id}/pause", s.withSession(s.routePause))
	mux.HandleFunc("POST /sessions/{session_id}/players/{guild_id}/resume", s.withSession(s.routeResume))
	mux.HandleFunc("POST /sessions/{session_id}/players/{guild_id}/stop", s.withSession(s.routeStop))
	mux.HandleFunc("POST /sessions/{session_id}/players/{guild_id}/seek", s.withSession(s.routeSeek))
	mux.HandleFunc("PATCH /sessions/{session_id}/players/{guild_id}/volume", s.withSession(s.routeVolume))
	mux.HandleFunc("DELETE /sessions/{session_id}/players/{guild_id}", s.withSession(s.routeDisconnect))
}

type sessionHandler func(client *Client, guildID snowflake.ID, w http.ResponseWriter, r *http.Request)

func (s *Server) withSession(next sessionHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("session_id")
		guildIDStr := r.PathValue("guild_id")

		client := s.getClientBySession(sessionID)
		if client == nil {
			writeJSON(w, http.StatusNotFound, protocol.ErrorResponse{Error: "session not found"})
			return
		}

		guildID, err := snowflake.Parse(guildIDStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, protocol.ErrorResponse{Error: "invalid guild_id"})
			return
		}

		next(client, guildID, w, r)
	}
}

func (s *Server) routePlay(client *Client, guildID snowflake.ID, w http.ResponseWriter, r *http.Request) {
	var play protocol.RequestPlay
	if err := json.NewDecoder(r.Body).Decode(&play); err != nil {
		writeJSON(w, http.StatusBadRequest, protocol.ErrorResponse{Error: "invalid request body"})
		return
	}

	player := client.getPlayer(guildID)
	if play.Volume > 0 {
		player.SetVolume(play.Volume)
	}

	s.logger.Info("play requested",
		slog.String("guild_id", guildID.String()),
		slog.String("url", play.URL),
	)

	source, err := s.voiceManager.Play(context.Background(), client.sessionID, guildID, play.URL, play.StartTime)
	if err != nil {
		s.logger.Error("playback failed", slog.Any("error", err))
		writeJSON(w, http.StatusInternalServerError, protocol.ErrorResponse{Error: err.Error()})
		return
	}

	player.SetPlayingState(play.URL, play.StartTime)

	state, position, volume := player.GetPlayerUpdateData()
	client.send(protocol.Message{
		Op: protocol.OpTrackStart,
		Data: protocol.TrackStartData{
			GuildID: guildID,
			Track: protocol.TrackInfo{
				URL:      source.URL(),
				Duration: source.Duration(),
			},
		},
	})

	client.send(protocol.Message{
		Op: protocol.OpPlayerUpdate,
		Data: protocol.PlayerUpdateData{
			GuildID:  guildID,
			State:    state,
			Position: position,
			Volume:   volume,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) routePause(client *Client, guildID snowflake.ID, w http.ResponseWriter, _ *http.Request) {
	player := client.getPlayer(guildID)
	if player == nil {
		writeJSON(w, http.StatusNotFound, protocol.ErrorResponse{Error: "player not found"})
		return
	}

	if err := s.voiceManager.Pause(client.sessionID, guildID); err != nil {
		s.logger.Error("failed to pause", slog.Any("error", err))
		writeJSON(w, http.StatusInternalServerError, protocol.ErrorResponse{Error: err.Error()})
		return
	}

	player.SetPausedState(s.voiceManager.Position(client.sessionID, guildID))

	state, position, volume := player.GetPlayerUpdateData()
	client.send(protocol.Message{
		Op: protocol.OpPlayerUpdate,
		Data: protocol.PlayerUpdateData{
			GuildID:  guildID,
			State:    state,
			Position: position,
			Volume:   volume,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) routeResume(client *Client, guildID snowflake.ID, w http.ResponseWriter, _ *http.Request) {
	player := client.getPlayer(guildID)
	if player == nil {
		writeJSON(w, http.StatusNotFound, protocol.ErrorResponse{Error: "player not found"})
		return
	}

	if err := s.voiceManager.Resume(client.sessionID, guildID); err != nil {
		s.logger.Error("failed to resume", slog.Any("error", err))
		writeJSON(w, http.StatusInternalServerError, protocol.ErrorResponse{Error: err.Error()})
		return
	}

	player.SetState(protocol.PlayerStatePlaying)
	player.SetStartedAt(time.Now())
	player.SetPosition(s.voiceManager.Position(client.sessionID, guildID))

	state, position, volume := player.GetPlayerUpdateData()
	client.send(protocol.Message{
		Op: protocol.OpPlayerUpdate,
		Data: protocol.PlayerUpdateData{
			GuildID:  guildID,
			State:    state,
			Position: position,
			Volume:   volume,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) routeStop(client *Client, guildID snowflake.ID, w http.ResponseWriter, _ *http.Request) {
	player := client.getPlayer(guildID)
	if player == nil {
		writeJSON(w, http.StatusNotFound, protocol.ErrorResponse{Error: "player not found"})
		return
	}

	if err := s.voiceManager.Stop(client.sessionID, guildID); err != nil {
		s.logger.Error("failed to stop", slog.Any("error", err))
		writeJSON(w, http.StatusInternalServerError, protocol.ErrorResponse{Error: err.Error()})
		return
	}

	player.SetIdleState()

	state, _, volume := player.GetPlayerUpdateData()
	client.send(protocol.Message{
		Op: protocol.OpPlayerUpdate,
		Data: protocol.PlayerUpdateData{
			GuildID:  guildID,
			State:    state,
			Position: 0,
			Volume:   volume,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) routeSeek(client *Client, guildID snowflake.ID, w http.ResponseWriter, r *http.Request) {
	var seek protocol.RequestSeek
	if err := json.NewDecoder(r.Body).Decode(&seek); err != nil {
		writeJSON(w, http.StatusBadRequest, protocol.ErrorResponse{Error: "invalid request body"})
		return
	}

	player := client.getPlayer(guildID)
	if player == nil {
		writeJSON(w, http.StatusNotFound, protocol.ErrorResponse{Error: "player not found"})
		return
	}

	if err := s.voiceManager.Seek(client.sessionID, guildID, seek.Position); err != nil {
		s.logger.Error("failed to seek", slog.Any("error", err))

		if strings.Contains(err.Error(), "not supported") {
			writeJSON(w, http.StatusNotImplemented, protocol.ErrorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusInternalServerError, protocol.ErrorResponse{Error: err.Error()})
		return
	}

	player.SetPosition(s.voiceManager.Position(client.sessionID, guildID))
	player.SetStartedAt(time.Now())

	state, position, volume := player.GetPlayerUpdateData()
	client.send(protocol.Message{
		Op: protocol.OpPlayerUpdate,
		Data: protocol.PlayerUpdateData{
			GuildID:  guildID,
			State:    state,
			Position: position,
			Volume:   volume,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) routeVolume(client *Client, guildID snowflake.ID, w http.ResponseWriter, r *http.Request) {
	var vol protocol.RequestVolume
	if err := json.NewDecoder(r.Body).Decode(&vol); err != nil {
		writeJSON(w, http.StatusBadRequest, protocol.ErrorResponse{Error: "invalid request body"})
		return
	}

	player := client.getPlayer(guildID)
	if player == nil {
		writeJSON(w, http.StatusNotFound, protocol.ErrorResponse{Error: "player not found"})
		return
	}

	player.SetVolume(vol.Volume)

	state, _, volume := player.GetPlayerUpdateData()
	client.send(protocol.Message{
		Op: protocol.OpPlayerUpdate,
		Data: protocol.PlayerUpdateData{
			GuildID:  guildID,
			State:    state,
			Position: s.voiceManager.Position(client.sessionID, guildID),
			Volume:   volume,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) routeDisconnect(client *Client, guildID snowflake.ID, w http.ResponseWriter, _ *http.Request) {
	s.logger.Info("processing disconnect", slog.String("guild_id", guildID.String()))

	if err := s.voiceManager.Disconnect(client.sessionID, guildID); err != nil {
		s.logger.Error("failed to disconnect", slog.Any("error", err))
	}

	client.removePlayer(guildID)

	client.send(protocol.Message{
		Op: protocol.OpVoiceDisconnect,
		Data: protocol.VoiceDisconnectData{
			GuildID: guildID,
			Reason:  "requested",
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
