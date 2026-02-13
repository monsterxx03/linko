import ReactMarkdown from 'react-markdown';
import { memo } from 'react';

interface MarkdownContentProps {
  content: string;
  className?: string;
}

export const MarkdownContent = memo(function MarkdownContent({
  content,
  className = '',
}: MarkdownContentProps) {
  return (
    <div className={`markdown-content ${className}`}>
      <ReactMarkdown
        components={{
          code({ children, className, node, ...props }) {
            const isInline = !className;
            return (
              <code
                className={`${className || ''} ${isInline ? 'bg-slate-100 px-1.5 py-0.5 rounded text-sm font-mono text-pink-600' : 'block bg-slate-50 p-3 rounded-lg overflow-x-auto text-xs font-mono my-2'}`}
                {...props}
              >
                {children}
              </code>
            );
          },
          pre({ children }) {
            return <>{children}</>;
          },
          p({ children }) {
            return <p className="mb-2 last:mb-0">{children}</p>;
          },
          ul({ children }) {
            return (
              <ul className="list-disc list-inside mb-2 space-y-1">
                {children}
              </ul>
            );
          },
          ol({ children }) {
            return (
              <ol className="list-decimal list-inside mb-2 space-y-1">
                {children}
              </ol>
            );
          },
          li({ children }) {
            return <li className="text-sm">{children}</li>;
          },
          strong({ children }) {
            return (
              <strong className="font-semibold text-slate-800">
                {children}
              </strong>
            );
          },
          em({ children }) {
            return <em className="italic">{children}</em>;
          },
          a({ href, children }) {
            return (
              <a
                href={href}
                target="_blank"
                rel="noopener noreferrer"
                className="text-indigo-600 hover:underline"
              >
                {children}
              </a>
            );
          },
          blockquote({ children }) {
            return (
              <blockquote className="border-l-4 border-slate-200 pl-3 py-1 my-2 bg-slate-50 italic text-slate-600">
                {children}
              </blockquote>
            );
          },
          h1({ children }) {
            return <h1 className="text-lg font-bold mb-2">{children}</h1>;
          },
          h2({ children }) {
            return <h2 className="text-base font-bold mb-1.5">{children}</h2>;
          },
          h3({ children }) {
            return <h3 className="text-sm font-bold mb-1">{children}</h3>;
          },
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
});
