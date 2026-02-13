import { useState, useCallback } from 'react';

export interface CopyButtonProps {
  text: string;
  label?: string;
  className?: string;
  title?: string;
}

export function CopyButton({ text, label = 'Copy', className = '', title = 'Copy to clipboard' }: CopyButtonProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Silently fail if clipboard is not available
    }
  }, [text]);

  return (
    <button
      onClick={handleCopy}
      className={`text-xs px-2.5 py-1 rounded-md border shadow-sm transition-all duration-200
        ${copied
          ? 'bg-green-50 border-green-300 text-green-700'
          : 'bg-white border-bg-300 text-bg-700 hover:bg-bg-50 hover:border-bg-400'
        } ${className}`}
      title={title}
    >
      {copied ? '✓ Copied!' : label}
    </button>
  );
}
