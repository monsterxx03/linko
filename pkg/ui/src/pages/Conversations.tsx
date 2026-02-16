import { memo, useCallback, useRef, useEffect, useState } from 'react';
import { MessageBubble } from '../components/MessageBubble';
import { useLLMConversation } from '../hooks/useLLMConversation';
import { Conversation } from '../contexts/SSEContext';

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60000).toFixed(1)}m`;
}

const StatusBadge = memo(({ status, model }: { status: Conversation['status']; model?: string }) => (
  <div className="flex items-center gap-2 mb-1">
    {status === 'streaming' ? (
      <span className="flex items-center gap-1">
        <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
        <span className="text-xs text-emerald-600">Streaming</span>
      </span>
    ) : status === 'error' ? (
      <span className="flex items-center gap-1">
        <span className="w-2 h-2 rounded-full bg-red-500" />
        <span className="text-xs text-red-600">Error</span>
      </span>
    ) : (
      <span className="text-xs text-bg-400">Done</span>
    )}
    {model && (
      <span className="text-xs px-1.5 py-0.5 bg-bg-200 text-bg-600 rounded">
        {model}
      </span>
    )}
  </div>
));

StatusBadge.displayName = 'StatusBadge';

const ConversationList = memo(function ConversationList({
  conversations,
  currentId,
  onSelect,
  isCollapsed,
  onCollapse
}: {
  conversations: Conversation[];
  currentId: string | null;
  onSelect: (id: string) => void;
  isCollapsed?: boolean;
  onCollapse?: () => void;
}) {
  const handleSelect = useCallback((id: string) => onSelect(id), [onSelect]);

  // Collapsed view: just show icons for each conversation
  if (isCollapsed) {
    return (
      <div className="flex flex-col items-center py-2 h-full">
        {onCollapse && (
          <button
            type="button"
            onClick={onCollapse}
            className="p-2 hover:bg-bg-100 rounded-lg transition-colors mb-2"
            title="Expand sidebar"
          >
            <svg className="w-4 h-4 text-bg-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 5l7 7-7 7M5 5l7 7-7 7" />
            </svg>
          </button>
        )}
        <div className="flex-1 overflow-y-auto">
          {conversations.map((conv) => (
            <button
              key={conv.id}
              type="button"
              onClick={() => handleSelect(conv.id)}
              className={`w-10 h-10 mb-2 rounded-lg flex items-center justify-center transition-colors ${
                conv.id === currentId ? 'bg-bg-200' : 'hover:bg-bg-100'
              }`}
              title={conv.messages[conv.messages.length - 1]?.content?.[0] || 'Empty'}
            >
              {conv.status === 'streaming' ? (
                <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
              ) : conv.status === 'error' ? (
                <span className="w-2 h-2 rounded-full bg-red-500" />
              ) : (
                <svg className="w-4 h-4 text-bg-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z" />
                </svg>
              )}
            </button>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-bg-200">
        <span className="text-xs font-medium text-bg-500 uppercase tracking-wide">Conversations</span>
        <button
          type="button"
          onClick={onCollapse}
          className="p-1 hover:bg-bg-100 rounded transition-colors"
          title="Collapse sidebar"
        >
          <svg className="w-4 h-4 text-bg-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 19l-7-7 7-7m8 14l-7-7 7-7" />
          </svg>
        </button>
      </div>

      {/* List */}
      <div className="flex-1 overflow-y-auto">
        {conversations.length === 0 ? (
          <div className="p-6 text-center">
            <svg className="w-12 h-12 mx-auto text-bg-300 mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z" />
            </svg>
            <p className="text-sm text-bg-500">No conversations yet</p>
            <p className="text-xs text-bg-400 mt-1">Start a chat with an LLM to see it here</p>
          </div>
        ) : (
          <ul className="divide-y divide-bg-100">
      {conversations.map((conv) => {
        const isSelected = conv.id === currentId;
        const lastMessage = conv.messages[conv.messages.length - 1];
        const duration = Date.now() - conv.started_at;

        return (
          <li key={conv.id}>
            <button
              type="button"
              onClick={() => handleSelect(conv.id)}
              className={`w-full p-4 text-left hover:bg-bg-50 transition-colors ${
                isSelected ? 'bg-bg-100' : ''
              }`}
            >
              <StatusBadge status={conv.status} model={conv.model} />

              <p className="text-xs text-bg-400 font-mono truncate mb-1" title={conv.id}>
                {conv.id}
              </p>

              <p className="text-sm text-bg-800 truncate mb-1">
                {lastMessage?.content?.[0] || 'Empty conversation'}
              </p>

              <div className="flex items-center gap-3 text-xs text-bg-400">
                <span>{conv.messages.length} messages</span>
                <span>{conv.total_tokens} tokens</span>
                <span>{formatDuration(duration)}</span>
              </div>
            </button>
          </li>
        );
      })}
    </ul>
        )}
      </div>
    </div>
  );
});

const ConversationView = memo(function ConversationView({
  conversation
}: {
  conversation: Conversation | undefined;
}) {
  const messagesRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<number>(0);
  const prevConvIdRef = useRef<string>('');
  const prevLastMessageRef = useRef<string>('');
  const isAtBottomRef = useRef(true);

  const [showNewMessageHint, setShowNewMessageHint] = useState(false);

  // 记录滚动位置
  const handleScroll = useCallback(() => {
    if (messagesRef.current) {
      const { scrollTop, scrollHeight, clientHeight } = messagesRef.current;
      const isAtBottom = scrollHeight - scrollTop - clientHeight <= 50;
      isAtBottomRef.current = isAtBottom;
      if (isAtBottom) {
        setShowNewMessageHint(false);
      }
      if (!isAtBottom) {
        scrollRef.current = scrollTop;
      }
    }
  }, []);

  // 切换 conversation 时恢复滚动位置
  useEffect(() => {
    const currentConvId = conversation?.id || '';
    if (currentConvId !== prevConvIdRef.current) {
      scrollRef.current = 0;
      prevConvIdRef.current = currentConvId;
      prevLastMessageRef.current = '';
      setShowNewMessageHint(false);
      if (messagesRef.current) {
        messagesRef.current.scrollTop = 0;
      }
    }
  }, [conversation]);

  // 检测新消息/内容更新
  useEffect(() => {
    if (!conversation || conversation.messages.length === 0) return;

    const lastMessage = conversation.messages[conversation.messages.length - 1];
    const lastContent = lastMessage.content.join('');

    // 有新内容且用户不在底部
    if (lastContent !== prevLastMessageRef.current && !isAtBottomRef.current) {
      setShowNewMessageHint(true);
    }
    prevLastMessageRef.current = lastContent;
  }, [conversation]);

  // 滚动到底部
  const scrollToBottom = useCallback(() => {
    if (messagesRef.current) {
      messagesRef.current.scrollTop = messagesRef.current.scrollHeight;
      setShowNewMessageHint(false);
    }
  }, []);

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
      <div className="px-6 py-4 border-b border-bg-200 bg-white shrink-0">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="font-semibold text-bg-800">
              {conversation.model || 'Conversation'}
            </h2>
            <div className="flex items-center gap-3 mt-1 text-xs text-bg-400">
              <span>{conversation.messages.length} messages</span>
              <span>{conversation.total_tokens} tokens</span>
              <span className={`capitalize ${
                conversation.status === 'error' ? 'text-red-600' :
                conversation.status === 'streaming' ? 'text-emerald-600' : ''
              }`}>
                {conversation.status === 'error' ? 'Error' : conversation.status}
              </span>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {conversation.status === 'streaming' && (
              <span className="flex items-center gap-1.5 px-2 py-1 bg-emerald-50 text-emerald-700 rounded-full text-xs font-medium">
                <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
                Live
              </span>
            )}
            {conversation.status === 'error' && (
              <span className="flex items-center gap-1.5 px-2 py-1 bg-red-50 text-red-700 rounded-full text-xs font-medium">
                <span className="w-2 h-2 rounded-full bg-red-500" />
                Error
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Messages */}
      <div
        ref={messagesRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto p-6 space-y-4"
      >
        {conversation.messages.map((msg) => (
          <MessageBubble
            key={msg.id}
            id={msg.id}
            role={msg.role as 'user' | 'assistant' | 'system' | 'tool'}
            content={msg.content}
            tokens={msg.tokens}
            tool_calls={msg.tool_calls}
            streaming_tool_calls={msg.streaming_tool_calls}
            tool_results={msg.tool_results}
            timestamp={msg.timestamp}
            isStreaming={msg.is_streaming}
            system_prompts={msg.system_prompts}
            tools={msg.tools}
            thinking={msg.thinking}
          />
        ))}
      </div>

      {/* 新消息提示气泡 */}
      {showNewMessageHint && (
        <button
          type="button"
          onClick={scrollToBottom}
          className="fixed bottom-8 left-1/2 -translate-x-1/2 px-4 py-2 bg-emerald-500 text-white rounded-full shadow-lg hover:bg-emerald-600 transition-colors flex items-center gap-2"
        >
          <span className="w-2 h-2 bg-white rounded-full animate-pulse" />
          <span>新消息</span>
          {conversation?.status === 'streaming' && <span className="text-xs opacity-75">(streaming)</span>}
        </button>
      )}
    </div>
  );
});

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

  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  const currentConversation = conversations.find((c: Conversation) => c.id === currentConversationId);

  const handleClear = useCallback(() => clear(), [clear]);
  const handleReconnect = useCallback(() => reconnect(), [reconnect]);
  const handleSelect = useCallback((id: string) => setCurrentConversationId(id), [setCurrentConversationId]);

  return (
    <div className="flex-1 flex flex-col h-[calc(100vh-180px)]">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold text-bg-800">LLM Conversations</h2>
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
            type="button"
            onClick={handleClear}
            className="px-3 py-1.5 text-sm text-bg-600 hover:text-bg-800 hover:bg-bg-100 rounded-lg transition-colors"
          >
            Clear
          </button>
          <button
            type="button"
            onClick={handleReconnect}
            className="px-3 py-1.5 text-sm text-bg-600 hover:text-bg-800 hover:bg-bg-100 rounded-lg transition-colors"
          >
            Reconnect
          </button>
        </div>
      </div>

      {/* Main content */}
      <div className="flex-1 flex border border-bg-200 rounded-xl bg-white h-full">
        {/* Conversation list */}
        <div
          className={`border-r border-bg-200 overflow-y-auto flex-shrink-0 transition-all duration-200 h-full ${
            sidebarCollapsed ? 'w-16' : 'w-80'
          }`}
        >
          <ConversationList
            conversations={conversations}
            currentId={currentConversationId}
            onSelect={handleSelect}
            isCollapsed={sidebarCollapsed}
            onCollapse={() => setSidebarCollapsed(!sidebarCollapsed)}
          />
        </div>


        {/* Conversation view */}
        <div className="flex-1 h-full">
          <ConversationView
            conversation={currentConversation}
          />
        </div>
      </div>
    </div>
  );
}
