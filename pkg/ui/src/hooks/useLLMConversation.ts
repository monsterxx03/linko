import { useState, useEffect, useCallback, useRef } from 'react';

// LLM Message types
export interface LLMMessage {
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string;
  name?: string;
  tool_calls?: ToolCall[];
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

// Event types from backend
export interface LLMMessageEvent {
  id: string;
  timestamp: string;
  conversation_id: string;
  request_id: string;
  message: LLMMessage;
  token_count?: number;
  total_tokens?: number;
  model?: string;
}

export interface LLMTokenEvent {
  id: string;
  timestamp: string;
  conversation_id: string;
  request_id: string;
  delta: string;
  is_complete: boolean;
  stop_reason?: string;
}

export interface ConversationUpdateEvent {
  id: string;
  timestamp: string;
  conversation_id: string;
  status: 'streaming' | 'complete' | 'error';
  message_count: number;
  total_tokens: number;
  duration_ms: number;
  model?: string;
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

export interface Message {
  id: string;
  role: string;
  content: string;
  tool_calls?: ToolCall[];
  tokens?: number;
  timestamp: number;
  is_streaming?: boolean;
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

export function useLLMConversation(options: UseLLMConversationOptions = {}): UseLLMConversationReturn {
  const { maxConversations = 50 } = options;
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [currentConversationId, setCurrentConversationId] = useState<string | null>(null);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const eventSourceRef = useRef<EventSource | null>(null);
  const conversationsRef = useRef<Map<string, Conversation>>(new Map());

  // Clear all conversations
  const clear = useCallback(() => {
    conversationsRef.current.clear();
    setConversations([]);
    setCurrentConversationId(null);
  }, []);

  // Reconnect to SSE
  const reconnect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
    }
    setIsConnected(false);
    setError(null);
  }, []);

  // Get or create conversation
  const getOrCreateConversation = useCallback((conversationId: string): Conversation => {
    const existing = conversationsRef.current.get(conversationId);
    if (existing) {
      return existing;
    }

    const newConversation: Conversation = {
      id: conversationId,
      status: 'streaming',
      messages: [],
      total_tokens: 0,
      started_at: Date.now(),
      last_updated: Date.now(),
    };
    conversationsRef.current.set(conversationId, newConversation);
    return newConversation;
  }, []);

  // Update conversation in state
  const updateConversation = useCallback((conversationId: string, update: Partial<Conversation>) => {
    const conv = getOrCreateConversation(conversationId);
    const updatedConv = { ...conv, ...update, last_updated: Date.now() };
    conversationsRef.current.set(conversationId, updatedConv);

    // Convert to array and limit size
    const convArray = Array.from(conversationsRef.current.values())
      .sort((a, b) => b.last_updated - a.last_updated)
      .slice(0, maxConversations);

    setConversations(convArray);
  }, [getOrCreateConversation, maxConversations]);

  // Handle message events
  const handleMessageEvent = useCallback((event: LLMMessageEvent) => {
    const conv = getOrCreateConversation(event.conversation_id);

    // Add or update message
    const messageIndex = conv.messages.findIndex(m => m.id === event.id);
    const newMessage: Message = {
      id: event.id,
      role: event.message.role,
      content: event.message.content,
      tool_calls: event.message.tool_calls,
      tokens: event.token_count,
      timestamp: new Date(event.timestamp).getTime(),
    };

    if (messageIndex >= 0) {
      conv.messages[messageIndex] = newMessage;
    } else {
      conv.messages.push(newMessage);
    }

    // Update conversation
    updateConversation(event.conversation_id, {
      messages: [...conv.messages],
      model: event.model || conv.model,
      total_tokens: event.total_tokens || conv.total_tokens,
    });

    // Set as current if first message
    if (conv.messages.length === 1) {
      setCurrentConversationId(event.conversation_id);
    }
  }, [getOrCreateConversation, updateConversation]);

  // Handle token events (streaming)
  const handleTokenEvent = useCallback((event: LLMTokenEvent) => {
    const conv = conversationsRef.current.get(event.conversation_id);
    if (!conv || conv.messages.length === 0) return;

    const lastMessage = conv.messages[conv.messages.length - 1];

    if (event.is_complete) {
      // Streaming complete
      lastMessage.is_streaming = false;
      updateConversation(event.conversation_id, {
        status: 'complete',
        messages: [...conv.messages],
      });
    } else {
      // Append token
      lastMessage.content += event.delta;
      lastMessage.is_streaming = true;
      updateConversation(event.conversation_id, {
        messages: [...conv.messages],
      });
    }
  }, [updateConversation]);

  // Handle conversation update
  const handleConversationUpdate = useCallback((event: ConversationUpdateEvent) => {
    updateConversation(event.conversation_id, {
      status: event.status,
      total_tokens: event.total_tokens,
      message_count: event.message_count,
      model: event.model,
    });
  }, [updateConversation]);

  useEffect(() => {
    const connectSSE = () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }

      const eventSource = new EventSource('/api/mitm/traffic/sse');
      eventSourceRef.current = eventSource;

      eventSource.addEventListener('welcome', () => {
        setIsConnected(true);
        setError(null);
      });

      eventSource.addEventListener('llm_message', (event) => {
        try {
          const data = JSON.parse(event.data);
          handleMessageEvent(data);
        } catch (e) {
          console.error('Failed to parse llm_message event:', e);
        }
      });

      eventSource.addEventListener('llm_token', (event) => {
        try {
          const data = JSON.parse(event.data);
          handleTokenEvent(data);
        } catch (e) {
          console.error('Failed to parse llm_token event:', e);
        }
      });

      eventSource.addEventListener('conversation', (event) => {
        try {
          const data = JSON.parse(event.data);
          handleConversationUpdate(data);
        } catch (e) {
          console.error('Failed to parse conversation event:', e);
        }
      });

      eventSource.addEventListener('error', () => {
        setIsConnected(false);
        if (eventSource.readyState === EventSource.CLOSED) {
          setError('Connection closed. Reconnecting...');
          setTimeout(connectSSE, 3000);
        }
      });
    };

    connectSSE();

    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
    };
  }, [handleMessageEvent, handleTokenEvent, handleConversationUpdate]);

  return {
    conversations,
    currentConversationId,
    setCurrentConversationId,
    isConnected,
    error,
    clear,
    reconnect,
  };
}
