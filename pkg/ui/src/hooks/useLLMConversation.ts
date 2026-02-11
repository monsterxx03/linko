import { useState, useEffect, useCallback } from 'react';
import { useSSEContext, Conversation as ConversationType } from '../contexts/SSEContext';

// Hook options
export interface UseLLMConversationOptions {
  maxConversations?: number;
}

// Hook return type
export interface UseLLMConversationReturn {
  conversations: ConversationType[];
  currentConversationId: string | null;
  setCurrentConversationId: (id: string | null) => void;
  isConnected: boolean;
  error: string | null;
  clear: () => void;
  reconnect: () => void;
}

export function useLLMConversation(_options: UseLLMConversationOptions = {}): UseLLMConversationReturn {
  const { llmConversations$, llmCurrentId$, isLLMConnected, clearLLM } = useSSEContext();
  const [conversations, setConversations] = useState<ConversationType[]>([]);
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

  // Subscribe to conversations from SSEContext
  useEffect(() => {
    const unsubConversations = llmConversations$.subscribe((allConversations: ConversationType[]) => {
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
