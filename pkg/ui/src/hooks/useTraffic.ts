import { useState, useEffect, useCallback, useRef } from 'react';

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

export interface UseTrafficOptions {
  maxEvents?: number;
  autoScroll?: boolean;
}

export interface UseTrafficReturn {
  events: TrafficEvent[];
  isConnected: boolean;
  error: string | null;
  filter: string;
  search: string;
  setFilter: (filter: string) => void;
  setSearch: (search: string) => void;
  setAutoScroll: (autoScroll: boolean) => void;
  clear: () => void;
  reconnect: () => void;
}

export function useTraffic(options: UseTrafficOptions = {}): UseTrafficReturn {
  const { maxEvents = 100, autoScroll: initialAutoScroll = true } = options;
  const [events, setEvents] = useState<TrafficEvent[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState('all');
  const [search, setSearch] = useState('');
  const [autoScroll, setAutoScroll] = useState(initialAutoScroll);
  const eventSourceRef = useRef<EventSource | null>(null);
  const eventsMapRef = useRef<Map<string, TrafficEvent>>(new Map());
  const orderRef = useRef<string[]>([]);

  const clear = useCallback(() => {
    eventsMapRef.current.clear();
    orderRef.current = [];
    setEvents([]);
  }, []);

  const reconnect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
    }
    setIsConnected(false);
    setError(null);
  }, []);

  const getFilteredEvents = useCallback((): TrafficEvent[] => {
    return orderRef.current
      .map((id) => eventsMapRef.current.get(id))
      .filter((e): e is TrafficEvent => e !== undefined)
      .filter((event) => {
        // Filter
        if (filter === 'requests' && event.direction !== 'client->server') {
          return false;
        }
        if (filter === 'responses' && event.direction !== 'server->client') {
          return false;
        }
        // Search
        if (search) {
          const eventStr = JSON.stringify(event).toLowerCase();
          if (!eventStr.includes(search.toLowerCase())) {
            return false;
          }
        }
        return true;
      });
  }, [filter, search]);

  const handleTrafficEvent = useCallback((trafficEvent: TrafficEvent) => {
    const eventId = trafficEvent.id;
    if (!eventId) return;

    const existing = eventsMapRef.current.get(eventId);
    const isNew = !existing;

    // Merge event data
    const mergedEvent: TrafficEvent = existing
      ? {
          ...existing,
          ...trafficEvent,
          request: { ...existing.request, ...trafficEvent.request },
          response: { ...existing.response, ...trafficEvent.response },
        }
      : trafficEvent;

    eventsMapRef.current.set(eventId, mergedEvent);

    if (isNew) {
      orderRef.current.unshift(eventId);
      if (orderRef.current.length > maxEvents) {
        const removedId = orderRef.current.pop();
        if (removedId) {
          eventsMapRef.current.delete(removedId);
        }
      }
    }

    setEvents(getFilteredEvents());
  }, [maxEvents, getFilteredEvents]);

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

      eventSource.addEventListener('traffic', (event) => {
        const trafficEvent = JSON.parse(event.data);
        handleTrafficEvent(trafficEvent);
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
  }, [handleTrafficEvent]);

  // Auto-scroll to top when new events arrive
  useEffect(() => {
    if (autoScroll && events.length > 0) {
      const container = document.getElementById('mitm-traffic-list');
      if (container) {
        container.scrollTop = 0;
      }
    }
  }, [events, autoScroll]);

  return {
    events: getFilteredEvents(),
    isConnected,
    error,
    filter,
    search,
    setFilter,
    setSearch,
    setAutoScroll,
    clear,
    reconnect,
  };
}
