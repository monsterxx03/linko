import { useState, useEffect, useCallback, useRef } from 'react';

// LLM Message types
export interface LLMMessage {
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string[];
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
  thinking?: string;
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

export interface ToolDef {
  name: string;
  description?: string;
  input_schema: Record<string, unknown>;
}

export interface Message {
  id: string;
  role: string;
  content: string[];
  tool_calls?: ToolCall[];
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

export function useLLMConversation(options: UseLLMConversationOptions = {}): UseLLMConversationReturn {
  const { maxConversations = 50 } = options;
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [currentConversationId, setCurrentConversationId] = useState<string | null>(null);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const eventSourceRef = useRef<EventSource | null>(null);
  const conversationsRef = useRef<Map<string, Conversation>>(new Map());
  // 用于跟踪当前活跃的对话 ID，处理追问时的 ID 变化
  const activeConversationIdRef = useRef<string | null>(null);

  // Clear all conversations
  const clear = useCallback(() => {
    conversationsRef.current.clear();
    setConversations([]);
    setCurrentConversationId(null);
    activeConversationIdRef.current = null;
  }, []);

  // Reconnect to SSE
  const reconnect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
    }
    setIsConnected(false);
    setError(null);
    activeConversationIdRef.current = null;
  }, []);

  // Get or create conversation
  const getOrCreateConversation = useCallback((conversationId: string): Conversation => {
    // Defensive check: ensure conversationId is not empty
    if (!conversationId) {
      // Generate a temporary unique ID to avoid key collisions
      conversationId = `temp-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
      console.warn('Empty conversationId provided, generated temporary ID:', conversationId);
    }

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
  const handleMessageEvent = useCallback((event: any) => {
    // Extract actual event data from extra field if present
    const actualEvent = event.extra || event;

    // Defensive check: ensure message exists
    if (!actualEvent.message) {
      console.warn('Received llm_message event without message field:', event);
      return;
    }

    // Defensive check: ensure conversation_id exists
    if (!actualEvent.conversation_id) {
      console.warn('Received llm_message event without conversation_id:', event);
      return;
    }

    // 检测对话 ID 是否变化（用户追问时会变化）
    const prevConversationId = activeConversationIdRef.current;
    const newConversationId = actualEvent.conversation_id;

    // 如果是新的对话（ID 变化了），将旧对话标记为完成
    if (prevConversationId && prevConversationId !== newConversationId) {
      const prevConv = conversationsRef.current.get(prevConversationId);
      if (prevConv && prevConv.status !== 'complete') {
        prevConv.status = 'complete';
        conversationsRef.current.set(prevConversationId, prevConv);
        // 触发状态更新
        setConversations(Array.from(conversationsRef.current.values()));
      }
    }

    // 获取或创建对话
    const conv = getOrCreateConversation(newConversationId);

    // Add or update message
    const messageIndex = conv.messages.findIndex(m => m.id === actualEvent.id);
    const newMessage: Message = {
      id: actualEvent.id,
      role: actualEvent.message.role || 'unknown',
      content: Array.isArray(actualEvent.message.content)
        ? actualEvent.message.content
        : typeof actualEvent.message.content === 'string'
          ? [actualEvent.message.content]
          : [],
      tool_calls: actualEvent.message.tool_calls,
      tokens: actualEvent.token_count,
      timestamp: new Date(actualEvent.timestamp).getTime(),
      system_prompts: actualEvent.message.system,
      tools: actualEvent.message.tools,
    };

    if (messageIndex >= 0) {
      conv.messages[messageIndex] = newMessage;
    } else {
      conv.messages.push(newMessage);
    }

    // Update conversation
    updateConversation(newConversationId, {
      messages: [...conv.messages],
      model: actualEvent.model || conv.model,
      total_tokens: actualEvent.total_tokens || conv.total_tokens,
    });

    // 设置当前活跃对话 ID，并自动选中
    activeConversationIdRef.current = newConversationId;
    setCurrentConversationId(newConversationId);
  }, [getOrCreateConversation, updateConversation]);

  // Handle token events (streaming)
  const handleTokenEvent = useCallback((event: any) => {
    // Extract actual event data from extra field if present
    const actualEvent = event.extra || event;

    // Defensive check: ensure conversation_id exists
    if (!actualEvent.conversation_id) {
      console.warn('Received llm_token event without conversation_id:', event);
      return;
    }

    const conv = conversationsRef.current.get(actualEvent.conversation_id);
    if (!conv || conv.messages.length === 0) return;

    const lastMessage = conv.messages[conv.messages.length - 1];

    if (actualEvent.is_complete) {
      // Streaming complete
      lastMessage.is_streaming = false;
      updateConversation(actualEvent.conversation_id, {
        status: 'complete',
        messages: [...conv.messages],
      });
      // 流结束时清除活跃对话标记
      if (activeConversationIdRef.current === actualEvent.conversation_id) {
        // 保留 ID，便于后续可能的追加，不立即清除
      }
    } else {
      // Append token
      if (lastMessage.content.length > 0) {
        lastMessage.content[lastMessage.content.length - 1] += actualEvent.delta;
      } else {
        lastMessage.content.push(actualEvent.delta);
      }
      // Handle thinking content (for Claude)
      if (actualEvent.thinking) {
        lastMessage.thinking = (lastMessage.thinking || '') + actualEvent.thinking;
      }
      lastMessage.is_streaming = true;
      updateConversation(actualEvent.conversation_id, {
        messages: [...conv.messages],
      });
    }
  }, [updateConversation]);

  // Handle conversation update
  const handleConversationUpdate = useCallback((event: any) => {
    // Extract actual event data from extra field if present
    const actualEvent = event.extra || event;
    
    // Defensive check: ensure conversation_id exists
    if (!actualEvent.conversation_id) {
      console.warn('Received conversation event without conversation_id:', event);
      return;
    }

    updateConversation(actualEvent.conversation_id, {
      status: actualEvent.status,
      total_tokens: actualEvent.total_tokens,
      message_count: actualEvent.message_count,
      model: actualEvent.model,
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

      eventSource.addEventListener('llm_error', (event) => {
        try {
          const data = JSON.parse(event.data);
          // Extract actual event data from extra field if present
          const actualEvent = data.extra || data;
          // Update conversation status to error
          if (actualEvent.conversation_id) {
            updateConversation(actualEvent.conversation_id, {
              status: 'error',
            });
          }
        } catch (e) {
          console.error('Failed to parse llm_error event:', e);
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
