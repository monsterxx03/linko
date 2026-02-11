import { useState, useEffect, useCallback } from 'react';
import { useSSEContext } from '../contexts/SSEContext';

// LLM Message types
export interface LLMMessage {
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string[];
  name?: string;
  tool_calls?: ToolCall[];
  system?: string[];
  tools?: ToolDef[];
}

export interface ToolCall {
  id: string;
  type: string;
  function: {
    name: string;
    arguments: string;
  };
}

export interface TokenUsage {
  input_tokens: number;
  output_tokens: number;
}

// Conversation state
export interface Conversation {
  id: string;
  model?: string;
  status: 'streaming' | 'complete' | 'error';
  messages: Message[];
  total_tokens: number;
  message_count?: number;
  started_at: number;
  last_updated: number;
}

export interface ToolDef {
  name: string;
  description?: string;
  input_schema: Record<string, unknown>;
}

export interface StreamingToolCall {
  id?: string;
  name?: string;
  arguments: string;
}

export interface Message {
  id: string;
  role: string;
  content: string[];
  tool_calls?: ToolCall[];
  streaming_tool_calls?: StreamingToolCall[];
  tokens?: number;
  timestamp: number;
  is_streaming?: boolean;
  system_prompts?: string[];
  tools?: ToolDef[];
  thinking?: string;
}

// Hook options
export interface UseLLMConversationOptions {
  maxConversations?: number;
}

// Hook return type
export interface UseLLMConversationReturn {
  conversations: Conversation[];
  currentConversationId: string | null;
  setCurrentConversationId: (id: string | null) => void;
  isConnected: boolean;
  error: string | null;
  clear: () => void;
  reconnect: () => void;
}

export function useLLMConversation(_options: UseLLMConversationOptions = {}): UseLLMConversationReturn {
  const { llmEvents$, llmConversations$, llmCurrentId$, isLLMConnected, clearLLM } = useSSEContext();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [currentConversationId, setCurrentConversationId] = useState<string | null>(null);
  const [isConnected, setIsConnected] = useState(false);

  // Sync connection status from context
  useEffect(() => {
    setIsConnected(isLLMConnected);
  }, [isLLMConnected]);

  // Clear all conversations
  const clear = useCallback(() => {
    setConversations([]);
    setCurrentConversationId(null);
    clearLLM();
  }, [clearLLM]);

  // Reconnect to SSE
  const reconnect = useCallback(() => {
    setIsConnected(false);
  }, []);

  // Subscribe to conversations from SSEContext (provides persisted data)
  useEffect(() => {
    const unsubConversations = llmConversations$.subscribe((allConversations: Conversation[]) => {
      setConversations([...allConversations]);
    });

    const unsubCurrentId = llmCurrentId$.subscribe((id: string | null) => {
      setCurrentConversationId(id);
    });

    return () => {
      unsubConversations();
      unsubCurrentId();
    };
  }, [llmConversations$, llmCurrentId$]);

  // Subscribe to LLM events for real-time updates (updates the store in SSEContext)
  useEffect(() => {
    const unsubMessage = llmEvents$.message.subscribe(() => {
      // Store update is handled in SSEProvider
    });

    const unsubToken = llmEvents$.token.subscribe(() => {
      // Store update is handled in SSEProvider
    });

    const unsubConversation = llmEvents$.conversation.subscribe(() => {
      // Store update is handled in SSEProvider
    });

    return () => {
      unsubMessage();
      unsubToken();
      unsubConversation();
    };
  }, [llmEvents$]);

  return {
    conversations,
    currentConversationId,
    setCurrentConversationId,
    isConnected,
    error: null,
    clear,
    reconnect,
  };
}
