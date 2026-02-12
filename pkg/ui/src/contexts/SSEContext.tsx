import React, {
  createContext,
  useContext,
  useEffect,
  useState,
  useMemo,
  useCallback,
} from "react";

// Traffic Event types
export interface TrafficEvent {
  id: string;
  timestamp: number;
  hostname?: string;
  direction?: string;
  connection_id?: string;
  request_id?: string;
  request?: {
    method?: string;
    url?: string;
    headers?: Record<string, string>;
    body?: string;
    content_type?: string;
  };
  response?: {
    status_code?: number;
    status?: string;
    headers?: Record<string, string>;
    body?: string;
    content_type?: string;
    latency?: number;
  };
}

// LLM Event types
export interface LLMMessage {
  role: "user" | "assistant" | "system" | "tool";
  content: string[];
  name?: string;
  tool_calls?: ToolCall[];
  system?: string[];
  tools?: ToolDef[];
}

export interface ToolDef {
  name: string;
  description?: string;
  input_schema: Record<string, unknown>;
}

export interface ToolCall {
  id: string;
  type: string;
  function: {
    name: string;
    arguments: string;
  };
}

export interface LLMMessageEvent {
  id: string;
  timestamp: string;
  conversation_id: string;
  message: LLMMessage;
  token_count?: number;
  total_tokens?: number;
  model?: string;
}

export interface LLMTokenEvent {
  id: string;
  conversation_id: string;
  delta: string;
  thinking?: string;
  tool_data?: string;
  tool_name?: string;
  tool_id?: string;
  is_complete: boolean;
  stop_reason?: string;
}

export interface ConversationUpdateEvent {
  id: string;
  timestamp: string;
  conversation_id: string;
  status: "streaming" | "complete" | "error";
  message_count: number;
  total_tokens: number;
  duration_ms: number;
  model?: string;
}

// Simple observable for events
type Subscriber<T> = (event: T) => void;

class EventObservable<T> {
  private subscribers: Set<Subscriber<T>> = new Set();
  private currentValue: T | null = null;
  private hasValue: boolean = false;

  subscribe(callback: Subscriber<T>): () => void {
    this.subscribers.add(callback);
    // Emit current value to new subscriber if available
    if (this.hasValue && this.currentValue !== null) {
      callback(this.currentValue);
    }
    return () => this.subscribers.delete(callback);
  }

  emit(event: T): void {
    this.currentValue = event;
    this.hasValue = true;
    this.subscribers.forEach((callback) => callback(event));
  }
}

// Single SSE connection that can emit multiple event types
class SharedSSEConnection {
  private eventSource: EventSource | null = null;
  private isConnected: boolean = false;
  private url: string = "";
  private eventHandlers: Map<string, (data: unknown) => void> = new Map();
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;

  connect(
    url: string,
    eventHandlers: Record<string, (data: unknown) => void>,
  ): void {
    this.url = url;
    this.eventHandlers = new Map(Object.entries(eventHandlers));

    if (this.eventSource) {
      this.eventSource.close();
    }

    this.eventSource = new EventSource(url);
    this.isConnected = false;

    // Handle welcome event
    this.eventSource.addEventListener("welcome", () => {
      this.isConnected = true;
    });

    // Handle registered event types
    this.eventHandlers.forEach((handler, eventName) => {
      this.eventSource?.addEventListener(eventName, (event) => {
        try {
          const data = JSON.parse(event.data);
          handler(data);
        } catch (e) {
          console.error(`Failed to parse ${eventName} event:`, e);
        }
      });
    });

    // Handle errors
    this.eventSource.addEventListener("error", () => {
      this.isConnected = false;
      if (this.eventSource?.readyState === EventSource.CLOSED) {
        this.scheduleReconnect();
      }
    });
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimeout) return;
    this.reconnectTimeout = setTimeout(() => {
      this.reconnectTimeout = null;
      this.connect(this.url, Object.fromEntries(this.eventHandlers));
    }, 3000);
  }

  disconnect(): void {
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
      this.isConnected = false;
    }
  }

  getConnectionStatus(): boolean {
    return this.isConnected;
  }
}

// Traffic event storage with observable
class TrafficEventStore {
  private eventsMap: Map<string, TrafficEvent> = new Map();
  private order: string[] = [];
  private maxEvents: number;
  private singleEvent$: EventObservable<TrafficEvent>;
  private allEvents$: EventObservable<TrafficEvent[]>;

  constructor(maxEvents: number = 100) {
    this.maxEvents = maxEvents;
    this.singleEvent$ = new EventObservable<TrafficEvent>();
    this.allEvents$ = new EventObservable<TrafficEvent[]>();
  }

  get singleEventObservable(): EventObservable<TrafficEvent> {
    return this.singleEvent$;
  }

  get allEventsObservable(): EventObservable<TrafficEvent[]> {
    return this.allEvents$;
  }

  getAll(): TrafficEvent[] {
    return this.order
      .map((id) => this.eventsMap.get(id))
      .filter((e): e is TrafficEvent => e !== undefined);
  }

  addEvent(event: TrafficEvent): void {
    const eventId = event.id;
    if (!eventId) return;

    const existing = this.eventsMap.get(eventId);
    const isNew = !existing;

    // Merge event data
    const mergedEvent: TrafficEvent = existing
      ? {
          ...existing,
          ...event,
          request: { ...existing.request, ...event.request },
          response: { ...existing.response, ...event.response },
        }
      : event;

    this.eventsMap.set(eventId, mergedEvent);

    if (isNew) {
      this.order.unshift(eventId);
      if (this.order.length > this.maxEvents) {
        const removedId = this.order.pop();
        if (removedId) {
          this.eventsMap.delete(removedId);
        }
      }
    }

    // Emit to subscribers
    this.singleEvent$.emit(mergedEvent);
    this.allEvents$.emit(this.getAll());
  }

  clear(): void {
    this.eventsMap.clear();
    this.order = [];
    this.allEvents$.emit([]);
  }
}

// Conversation state types (needed for LLM store)
export interface Conversation {
  id: string;
  model?: string;
  status: "streaming" | "complete" | "error";
  messages: Message[];
  total_tokens: number;
  message_count?: number;
  started_at: number;
  last_updated: number;
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

interface StreamingToolCall {
  id?: string;
  name?: string;
  arguments: string;
}

// LLM Conversation store with observable
class LLMConversationStore {
  private conversationsMap: Map<string, Conversation> = new Map();
  private maxConversations: number;
  private observable: EventObservable<Conversation[]>;
  private currentId$: EventObservable<string | null>;

  constructor(maxConversations: number = 50) {
    this.maxConversations = maxConversations;
    this.observable = new EventObservable<Conversation[]>();
    this.currentId$ = new EventObservable<string | null>();
  }

  get conversationsObservable(): EventObservable<Conversation[]> {
    return this.observable;
  }

  get currentIdObservable(): EventObservable<string | null> {
    return this.currentId$;
  }

  getAll(): Conversation[] {
    return Array.from(this.conversationsMap.values())
      .sort((a, b) => b.last_updated - a.last_updated)
      .slice(0, this.maxConversations);
  }

  getConversation(id: string): Conversation | undefined {
    return this.conversationsMap.get(id);
  }

  // Get or create conversation
  getOrCreate(conversationId: string): Conversation {
    const existing = this.conversationsMap.get(conversationId);
    if (existing) return existing;

    const newConversation: Conversation = {
      id: conversationId,
      status: "streaming",
      messages: [],
      total_tokens: 0,
      started_at: Date.now(),
      last_updated: Date.now(),
    };
    this.conversationsMap.set(conversationId, newConversation);
    return newConversation;
  }

  updateConversation(
    conversationId: string,
    update: Partial<Conversation>,
  ): void {
    const conv = this.getOrCreate(conversationId);
    const updatedConv = { ...conv, ...update, last_updated: Date.now() };
    this.conversationsMap.set(conversationId, updatedConv);
    this.observable.emit(this.getAll());
  }

  setCurrentId(id: string | null): void {
    this.currentId$.emit(id);
  }

  clear(): void {
    this.conversationsMap.clear();
    this.observable.emit([]);
    this.currentId$.emit(null);
  }
}

// LLM SSE connection with typed observables
class LLMSSEConnection {
  private connection: SharedSSEConnection;
  private messageEvents$: EventObservable<LLMMessageEvent>;
  private tokenEvents$: EventObservable<LLMTokenEvent>;
  private conversationEvents$: EventObservable<ConversationUpdateEvent>;

  constructor() {
    this.connection = new SharedSSEConnection();
    this.messageEvents$ = new EventObservable<LLMMessageEvent>();
    this.tokenEvents$ = new EventObservable<LLMTokenEvent>();
    this.conversationEvents$ = new EventObservable<ConversationUpdateEvent>();
  }

  connect(url: string): void {
    this.connection.connect(url, {
      llm_message: (data: unknown) => {
        this.messageEvents$.emit(data as LLMMessageEvent);
      },
      llm_token: (data: unknown) => {
        this.tokenEvents$.emit(data as LLMTokenEvent);
      },
      conversation: (data: unknown) => {
        this.conversationEvents$.emit(data as ConversationUpdateEvent);
      },
      welcome: () => {
        // Welcome is handled by SharedSSEConnection
      },
    });
  }

  disconnect(): void {
    this.connection.disconnect();
  }

  getConnectionStatus(): boolean {
    return this.connection.getConnectionStatus();
  }

  get observables() {
    return {
      message: this.messageEvents$,
      token: this.tokenEvents$,
      conversation: this.conversationEvents$,
    };
  }
}

// SSE Context interface
interface SSEContextType {
  trafficEvents$: EventObservable<TrafficEvent>;
  trafficAllEvents$: EventObservable<TrafficEvent[]>;
  clearTraffic: () => void;
  llmEvents$: {
    message: EventObservable<LLMMessageEvent>;
    token: EventObservable<LLMTokenEvent>;
    conversation: EventObservable<ConversationUpdateEvent>;
  };
  llmConversations$: EventObservable<Conversation[]>;
  llmCurrentId$: EventObservable<string | null>;
  clearLLM: () => void;
  isTrafficConnected: boolean;
  isLLMConnected: boolean;
}

const SSEContext = createContext<SSEContextType | null>(null);

interface SSEProviderProps {
  children: React.ReactNode;
}

export function SSEProvider({ children }: SSEProviderProps) {
  const [isTrafficConnected, setIsTrafficConnected] = useState(false);
  const [isLLMConnected, setIsLLMConnected] = useState(false);

  // Traffic event store (persists across re-renders)
  const trafficStore = useMemo(() => new TrafficEventStore(100), []);
  const trafficEvents$ = trafficStore.singleEventObservable;
  const trafficAllEvents$ = trafficStore.allEventsObservable;
  const clearTraffic = useCallback(() => trafficStore.clear(), [trafficStore]);

  // LLM conversation store (persists across re-renders)
  const llmStore = useMemo(() => new LLMConversationStore(50), []);
  const llmConversations$ = llmStore.conversationsObservable;
  const llmCurrentId$ = llmStore.currentIdObservable;
  const clearLLM = useCallback(() => llmStore.clear(), [llmStore]);

  // LLM connection with shared EventSource
  const llmConnection = useMemo(() => new LLMSSEConnection(), []);
  const llmEvents$ = useMemo(() => llmConnection.observables, [llmConnection]);

  // Connect to traffic SSE
  useEffect(() => {
    const trafficConnection = new SharedSSEConnection();

    trafficConnection.connect("/api/mitm/traffic/sse", {
      traffic: (data: unknown) => {
        trafficStore.addEvent(data as TrafficEvent);
      },
      welcome: () => {},
    });

    // Update connection status
    const checkConnection = setInterval(() => {
      setIsTrafficConnected(trafficConnection.getConnectionStatus());
    }, 1000);

    return () => {
      clearInterval(checkConnection);
      trafficConnection.disconnect();
    };
  }, [trafficStore]);

  // Connect to LLM conversation SSE (single connection)
  useEffect(() => {
    llmConnection.connect("/api/llm/conversation/sse");

    // Process LLM events and update store
    const unsubMessage = llmEvents$.message.subscribe(
      (event: LLMMessageEvent) => {
        if (!event.conversation_id || !event.message) return;

        const conv = llmStore.getOrCreate(event.conversation_id);
        const messageId = event.id;

        // Add or update message
        const messageIndex = conv.messages.findIndex((m) => m.id === messageId);
        if (messageIndex >= 0) {
          // Update existing
          const msg = conv.messages[messageIndex];
          msg.role = event.message.role || "unknown";

          // 只在有新内容时才更新 content，避免覆盖通过 llm_token 积累的内容
          const newContent = Array.isArray(event.message.content)
            ? event.message.content
            : typeof event.message.content === "string"
              ? [event.message.content]
              : [];
          // 只有当新内容非空时才更新
          if (
            newContent.length > 0 &&
            newContent.some((c) => c && c.trim() !== "")
          ) {
            msg.content = newContent;
          }

          // 只在有新 tool_calls 时才更新，避免覆盖通过 llm_token 积累的内容
          if (event.message.tool_calls && event.message.tool_calls.length > 0) {
            msg.tool_calls = event.message.tool_calls;
          }
          msg.tokens = event.token_count;
          msg.timestamp = new Date(event.timestamp).getTime();
          // 只在有新 system 时才更新
          if (event.message.system && event.message.system.length > 0) {
            msg.system_prompts = event.message.system;
          }
          // 只在有新 tools 时才更新
          if (event.message.tools && event.message.tools.length > 0) {
            msg.tools = event.message.tools;
          }
        } else {
          // Add new
          conv.messages.push({
            id: messageId,
            role: event.message.role || "unknown",
            content: Array.isArray(event.message.content)
              ? event.message.content
              : typeof event.message.content === "string"
                ? [event.message.content]
                : [],
            tool_calls: event.message.tool_calls,
            tokens: event.token_count,
            timestamp: new Date(event.timestamp).getTime(),
            system_prompts: event.message.system,
            tools: event.message.tools,
            thinking: "",
            streaming_tool_calls: [],
          });
        }

        llmStore.updateConversation(event.conversation_id, {
          messages: [...conv.messages],
          model: event.model || conv.model,
          total_tokens: event.total_tokens || conv.total_tokens,
        });
        llmStore.setCurrentId(event.conversation_id);
      },
    );

    const unsubToken = llmEvents$.token.subscribe((event: LLMTokenEvent) => {
      if (!event.conversation_id) return;

      const conv = llmStore.getConversation(event.conversation_id);
      if (!conv) return;

      const messageId = event.id;
      let messageIndex = conv.messages.findIndex((m) => m.id === messageId);
      let lastMessage = messageIndex >= 0 ? conv.messages[messageIndex] : null;

      if (!lastMessage) {
        lastMessage = {
          id: messageId,
          role: "assistant",
          content: [""],
          thinking: "",
          streaming_tool_calls: [],
          timestamp: Date.now(),
        };
        conv.messages.push(lastMessage);
        messageIndex = conv.messages.length - 1;
      }

      if (event.is_complete) {
        // Finalize
        if (
          lastMessage.streaming_tool_calls &&
          lastMessage.streaming_tool_calls.length > 0
        ) {
          lastMessage.tool_calls = lastMessage.streaming_tool_calls
            .filter((tc) => tc.id && tc.name)
            .map((tc) => ({
              id: tc.id!,
              type: "function" as const,
              function: { name: tc.name!, arguments: tc.arguments },
            }));
          lastMessage.streaming_tool_calls = undefined;
        }
        lastMessage.is_streaming = false;
        llmStore.updateConversation(event.conversation_id, {
          status: "complete",
          messages: [...conv.messages],
        });
      } else {
        // Tool call
        if (event.tool_name || event.tool_data || event.tool_id) {
          if (!lastMessage.streaming_tool_calls) {
            lastMessage.streaming_tool_calls = [];
          }
          let currentTool = event.tool_id
            ? lastMessage.streaming_tool_calls.find(
                (tc) => tc.id === event.tool_id,
              )
            : lastMessage.streaming_tool_calls[
                lastMessage.streaming_tool_calls.length - 1
              ];

          if (!currentTool && (event.tool_name || event.tool_id)) {
            currentTool = {
              id: event.tool_id || `temp_${Date.now()}`,
              name: event.tool_name,
              arguments: "",
            };
            lastMessage.streaming_tool_calls.push(currentTool);
          }
          if (event.tool_name && currentTool && !currentTool.name) {
            currentTool.name = event.tool_name;
          }
          if (event.tool_data && currentTool) {
            currentTool.arguments =
              (currentTool.arguments || "") + event.tool_data;
          }
        } else {
          // Text delta - 创建新的 content 数组以确保 React 检测到变化
          const lastIndex = lastMessage.content.length - 1;
          const newContent = [...lastMessage.content];
          if (lastIndex >= 0) {
            newContent[lastIndex] = (newContent[lastIndex] || "") + event.delta;
          } else {
            newContent.push(event.delta);
          }
          lastMessage.content = newContent;
        }

        if (event.thinking) {
          lastMessage.thinking = (lastMessage.thinking || "") + event.thinking;
        }
        lastMessage.is_streaming = true;

        llmStore.updateConversation(event.conversation_id, {
          messages: [...conv.messages],
        });
      }
    });

    const unsubConversation = llmEvents$.conversation.subscribe(
      (event: ConversationUpdateEvent) => {
        if (!event.conversation_id) return;
        llmStore.updateConversation(event.conversation_id, {
          status: event.status,
          total_tokens: event.total_tokens,
          message_count: event.message_count,
          model: event.model,
        });
      },
    );

    // Update connection status
    const checkConnection = setInterval(() => {
      setIsLLMConnected(llmConnection.getConnectionStatus());
    }, 1000);

    return () => {
      clearInterval(checkConnection);
      unsubMessage();
      unsubToken();
      unsubConversation();
      llmConnection.disconnect();
    };
  }, [llmConnection, llmEvents$, llmStore]);

  const value: SSEContextType = {
    trafficEvents$,
    trafficAllEvents$,
    clearTraffic,
    llmEvents$,
    llmConversations$,
    llmCurrentId$,
    clearLLM,
    isTrafficConnected,
    isLLMConnected,
  };

  return <SSEContext.Provider value={value}>{children}</SSEContext.Provider>;
}

export function useSSEContext(): SSEContextType {
  const context = useContext(SSEContext);
  if (!context) {
    throw new Error("useSSEContext must be used within SSEProvider");
  }
  return context;
}
