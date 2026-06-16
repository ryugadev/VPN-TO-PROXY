package event

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
	"vpn-to-proxy/internal/domain"
)

type CommandBus struct {
	cmdRepo    domain.AgentCommandRepository
	agentRepo  domain.AgentRepository
	wsRegistry *sync.Map // Map of agentID -> chan []byte (send channel)
	eventBus   *EventBus
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewCommandBus(cmdRepo domain.AgentCommandRepository, agentRepo domain.AgentRepository, eventBus *EventBus) *CommandBus {
	ctx, cancel := context.WithCancel(context.Background())
	bus := &CommandBus{
		cmdRepo:    cmdRepo,
		agentRepo:  agentRepo,
		wsRegistry: &sync.Map{},
		eventBus:   eventBus,
		ctx:        ctx,
		cancel:     cancel,
	}
	go bus.startTimeoutMonitor()
	return bus
}

func (b *CommandBus) RegisterAgentConnection(agentID string, sendChan chan<- []byte) {
	b.wsRegistry.Store(agentID, sendChan)
	log.Printf("Agent %s connection registered in CommandBus. Processing pending tasks...", agentID)
	go b.processPendingCommandsForAgent(agentID)
}

func (b *CommandBus) UnregisterAgentConnection(agentID string) {
	b.wsRegistry.Delete(agentID)
	log.Printf("Agent %s connection unregistered from CommandBus.", agentID)
}

func (b *CommandBus) Dispatch(cmd *domain.AgentCommand) error {
	// Check idempotency
	if cmd.IdempotencyKey != "" {
		existing, err := b.cmdRepo.GetByIdempotencyKey(cmd.IdempotencyKey)
		if err == nil && existing != nil {
			return fmt.Errorf("idempotency conflict: key %s already exists", cmd.IdempotencyKey)
		}
	}

	cmd.Status = "pending"
	cmd.CreatedAt = time.Now()
	if cmd.MaxAttempts == 0 {
		cmd.MaxAttempts = 3
	}
	if cmd.TimeoutSeconds == 0 {
		cmd.TimeoutSeconds = 120
	}

	if err := b.cmdRepo.Create(cmd); err != nil {
		return err
	}

	b.eventBus.Publish(Event{Type: "CommandCreated", AgentID: cmd.AgentID, TargetID: cmd.ID, Payload: cmd})
	go b.attemptDispatch(cmd)
	return nil
}

func (b *CommandBus) attemptDispatch(cmd *domain.AgentCommand) {
	sendChanVal, ok := b.wsRegistry.Load(cmd.AgentID)
	if !ok {
		log.Printf("Agent %s is offline. Command %s remains pending.", cmd.AgentID, cmd.ID)
		return
	}

	sendChan, ok := sendChanVal.(chan<- []byte)
	if !ok {
		return
	}

	now := time.Now()
	cmd.Status = "dispatched"
	cmd.DispatchedAt = &now
	cmd.Attempts++
	_ = b.cmdRepo.Update(cmd)

	msg := map[string]interface{}{
		"type":            "command",
		"command_id":      cmd.ID,
		"command_type":    cmd.Type,
		"payload":         cmd.Payload,
		"timeout_seconds": cmd.TimeoutSeconds,
		"idempotency_key": cmd.IdempotencyKey,
	}
	msgBytes, _ := json.Marshal(msg)

	// Attempt write
	select {
	case sendChan <- msgBytes:
		log.Printf("Dispatched command %s (%s) to agent %s", cmd.ID, cmd.Type, cmd.AgentID)
		b.eventBus.Publish(Event{Type: "CommandDispatched", AgentID: cmd.AgentID, TargetID: cmd.ID, Payload: cmd})
	default:
		log.Printf("Buffer full, failed to queue command %s for agent %s. Reverting to pending.", cmd.ID, cmd.AgentID)
		cmd.Status = "pending"
		_ = b.cmdRepo.Update(cmd)
	}
}

func (b *CommandBus) HandleResult(commandID string, status string, result string, lastError string) error {
	cmd, err := b.cmdRepo.GetByID(commandID)
	if err != nil {
		return err
	}

	if cmd.Status == "completed" || cmd.Status == "failed" || cmd.Status == "dead_letter" || cmd.Status == "timeout" {
		return nil // Already terminal
	}

	now := time.Now()

	if status == "executing" {
		cmd.Status = "executing"
		cmd.ExecutedAt = &now
		_ = b.cmdRepo.Update(cmd)
		b.eventBus.Publish(Event{Type: "CommandExecuting", AgentID: cmd.AgentID, TargetID: cmd.ID, Payload: cmd})
		return nil
	}

	if status == "completed" {
		cmd.Status = "completed"
		cmd.CompletedAt = &now
		cmd.Result = result
		_ = b.cmdRepo.Update(cmd)
		b.eventBus.Publish(Event{Type: CommandExecuted, AgentID: cmd.AgentID, TargetID: cmd.ID, Payload: cmd})
		return nil
	}

	if status == "failed" {
		cmd.LastError = lastError
		if cmd.Attempts < cmd.MaxAttempts {
			log.Printf("Command %s failed. Retrying (attempt %d/%d). Error: %s", cmd.ID, cmd.Attempts, cmd.MaxAttempts, lastError)
			cmd.Status = "pending"
			_ = b.cmdRepo.Update(cmd)
			time.AfterFunc(3*time.Second, func() {
				b.attemptDispatch(cmd)
			})
		} else {
			log.Printf("Command %s failed permanently. Marking as dead_letter. Error: %s", cmd.ID, lastError)
			cmd.Status = "dead_letter"
			_ = b.cmdRepo.Update(cmd)
			b.eventBus.Publish(Event{Type: ProxyFailed, AgentID: cmd.AgentID, TargetID: cmd.ID, Payload: cmd})
		}
		return nil
	}

	return fmt.Errorf("unknown status update: %s", status)
}

func (b *CommandBus) processPendingCommandsForAgent(agentID string) {
	cmds, err := b.cmdRepo.ListPendingByAgent(agentID)
	if err != nil {
		return
	}
	for _, cmd := range cmds {
		c := cmd
		b.attemptDispatch(&c)
	}
}

func (b *CommandBus) startTimeoutMonitor() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.checkCommandTimeouts()
		}
	}
}

func (b *CommandBus) checkCommandTimeouts() {
	active, err := b.cmdRepo.ListActive()
	if err != nil {
		return
	}

	now := time.Now()
	for _, cmd := range active {
		ref := cmd.CreatedAt
		if cmd.DispatchedAt != nil {
			ref = *cmd.DispatchedAt
		}
		if now.Sub(ref) > time.Duration(cmd.TimeoutSeconds)*time.Second {
			log.Printf("Command %s timed out.", cmd.ID)
			cmd.Status = "timeout"
			cmd.LastError = "execution timed out"
			_ = b.cmdRepo.Update(&cmd)
			b.eventBus.Publish(Event{Type: ProxyFailed, AgentID: cmd.AgentID, TargetID: cmd.ID, Payload: &cmd})
		}
	}
}

func (b *CommandBus) Close() {
	b.cancel()
}
