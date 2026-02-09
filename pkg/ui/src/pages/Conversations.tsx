import { useEffect, useRef } from 'react';
import { MessageBubble } from '../components/MessageBubble';
import { useLLMConversation, Conversation } from '../hooks/useLLMConversation';

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60000).toFixed(1)}m`;
}

function ConversationList({
  conversations,
  currentId,
  onSelect
}: {
  conversations: Conversation[];
  currentId: string | null;
  onSelect: (id: string) => void;
}) {
  if (conversations.length === 0) {
    return (
      <div className="p-6 text-center">
        <svg className="w-12 h-12 mx-auto text-bg-300 mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z" />
        </svg>
        <p className="text-sm text-bg-500">No conversations yet</p>
        <p className="text-xs text-bg-400 mt-1">Start a chat with an LLM to see it here</p>
      </div>
    );
  }

  return (
    <ul className="divide-y divide-bg-100">
      {conversations.map((conv) => {
        const isSelected = conv.id === currentId;
        const lastMessage = conv.messages[conv.messages.length - 1];

        return (
          <li key={conv.id}>
            <button
              onClick={() => onSelect(conv.id)}
              className={`w-full p-4 text-left hover:bg-bg-50 transition-colors ${
                isSelected ? 'bg-bg-100' : ''
              }`}
            >
              <div className="flex items-center gap-2 mb-1">
                {conv.status === 'streaming' ? (
                  <span className="flex items-center gap-1">
                    <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
                    <span className="text-xs text-emerald-600">Streaming</span>
                  </span>
                ) : (
                  <span className="text-xs text-bg-400">Done</span>
                )}
                {conv.model && (
                  <span className="text-xs px-1.5 py-0.5 bg-bg-200 text-bg-600 rounded">
                    {conv.model}
                  </span>
                )}
              </div>

              {/* Debug: 显示 conversation ID */}
              <p className="text-xs text-bg-400 font-mono truncate mb-1" title={conv.id}>
                {conv.id}
              </p>

              <p className="text-sm text-bg-800 truncate mb-1">
                {lastMessage?.content || 'Empty conversation'}
              </p>

              <div className="flex items-center gap-3 text-xs text-bg-400">
                <span>{conv.messages.length} messages</span>
                <span>{conv.total_tokens} tokens</span>
                <span>{formatDuration(Date.now() - conv.started_at)}</span>
              </div>
            </button>
          </li>
        );
      })}
    </ul>
  );
}

function ConversationView({
  conversation
}: {
  conversation: Conversation | undefined;
}) {
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [conversation?.messages]);

  if (!conversation) {
    return (
      <div className="flex-1 flex items-center justify-center text-bg-400">
        <div className="text-center">
          <svg className="w-16 h-16 mx-auto mb-4 text-bg-200" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z" />
          </svg>
          <p className="text-lg font-medium text-bg-500">Select a conversation</p>
          <p className="text-sm mt-1">Choose a conversation from the list to view messages</p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col h-full">
      {/* Header */}
      <div className="px-6 py-4 border-b border-bg-200 bg-white">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="font-semibold text-bg-800">
              {conversation.model || 'Conversation'}
            </h2>
            <div className="flex items-center gap-3 mt-1 text-xs text-bg-400">
              <span>{conversation.messages.length} messages</span>
              <span>{conversation.total_tokens} tokens</span>
              <span className="capitalize">{conversation.status}</span>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {conversation.status === 'streaming' && (
              <span className="flex items-center gap-1.5 px-2 py-1 bg-emerald-50 text-emerald-700 rounded-full text-xs font-medium">
                <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
                Live
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-6 space-y-4">
        {conversation.messages.map((msg) => (
          <MessageBubble
            key={msg.id}
            role={msg.role as 'user' | 'assistant' | 'system' | 'tool'}
            content={msg.content}
            tokens={msg.tokens}
            tool_calls={msg.tool_calls}
            timestamp={msg.timestamp}
            isStreaming={msg.is_streaming}
          />
        ))}
        <div ref={messagesEndRef} />
      </div>
    </div>
  );
}

export default function Conversations() {
  const {
    conversations,
    currentConversationId,
    setCurrentConversationId,
    isConnected,
    error,
    clear,
    reconnect,
  } = useLLMConversation();

  const currentConversation = conversations.find(c => c.id === currentConversationId);

  return (
    <div className="h-[calc(100vh-180px)] flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div>
          <h2 className="text-lg font-semibold text-bg-800">LLM Conversations</h2>
          <p className="text-sm text-bg-500">Monitor and inspect LLM API traffic</p>
        </div>

        <div className="flex items-center gap-3">
          {/* Connection status */}
          <div className="flex items-center gap-2">
            {isConnected ? (
              <>
                <span className="w-2 h-2 rounded-full bg-emerald-500" />
                <span className="text-xs text-bg-500">Connected</span>
              </>
            ) : (
              <>
                <span className="w-2 h-2 rounded-full bg-red-500" />
                <span className="text-xs text-red-500">{error || 'Disconnected'}</span>
              </>
            )}
          </div>

          <button
            onClick={clear}
            className="px-3 py-1.5 text-sm text-bg-600 hover:text-bg-800 hover:bg-bg-100 rounded-lg transition-colors"
          >
            Clear
          </button>
          <button
            onClick={reconnect}
            className="px-3 py-1.5 text-sm text-bg-600 hover:text-bg-800 hover:bg-bg-100 rounded-lg transition-colors"
          >
            Reconnect
          </button>
        </div>
      </div>

      {/* Main content */}
      <div className="flex-1 flex overflow-hidden border border-bg-200 rounded-xl bg-white">
        {/* Conversation list */}
        <div className="w-80 border-r border-bg-200 overflow-y-auto">
          <ConversationList
            conversations={conversations}
            currentId={currentConversationId}
            onSelect={setCurrentConversationId}
          />
        </div>

        {/* Conversation view */}
        <div className="flex-1 overflow-hidden">
          <ConversationView conversation={currentConversation} />
        </div>
      </div>
    </div>
  );
}
