import { useState, useEffect, useCallback } from 'react';
import { useSSEContext, TrafficEvent } from '../contexts/SSEContext';

export interface UseTrafficReturn {
  events: TrafficEvent[];
  isConnected: boolean;
  error: string | null;
  filter: string;
  search: string;
  setFilter: (filter: string) => void;
  setSearch: (search: string) => void;
  clear: () => void;
  reconnect: () => void;
}

export function useTraffic(): UseTrafficReturn {
  const { trafficAllEvents$, isTrafficConnected, clearTraffic } = useSSEContext();
  const [events, setEvents] = useState<TrafficEvent[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState('all');
  const [search, setSearch] = useState('');
  const [storedEvents, setStoredEvents] = useState<TrafficEvent[]>([]);

  // Sync connection status from context
  useEffect(() => {
    setIsConnected(isTrafficConnected);
  }, [isTrafficConnected]);

  const clear = useCallback(() => {
    setStoredEvents([]);
    setEvents([]);
    clearTraffic();
  }, [clearTraffic]);

  const reconnect = useCallback(() => {
    setIsConnected(false);
    setError(null);
  }, []);

  // Subscribe to all events from SSEContext
  useEffect(() => {
    const unsubscribe = trafficAllEvents$.subscribe((allEvents: TrafficEvent[]) => {
      setStoredEvents(allEvents);
    });

    return unsubscribe;
  }, [trafficAllEvents$]);

  // Filter events when storedEvents, filter, or search changes
  useEffect(() => {
    const filtered = storedEvents
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

    setEvents(filtered);
  }, [storedEvents, filter, search]);

  return {
    events,
    isConnected,
    error,
    filter,
    search,
    setFilter,
    setSearch,
    clear,
    reconnect,
  };
}
