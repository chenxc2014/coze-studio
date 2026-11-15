const paths: Record<string, string> = {
  '1001': 'M10 3a7 7 0 1 0 0 14 7 7 0 0 0 0-14m5 12 6 6',
  '1002': 'M4 4h16v16H4zM8 8h8M8 12h3m3 0h2m-8 4h8',
  '1003': 'M4 10h16v10H4zM3 10l3-6h12l3 6M9 20v-6h6v6',
  '1004': 'M3 4h18v16H3zM3 16l6-6 5 5 3-3 4 4M15 8h.01',
  '1005': 'M3 7h18v14H3zM8 7V3h8v4M3 12h18m-10 0v3h2v-3',
  '1006': 'M3 4h6l3 2 3-2h6v15h-6l-3 2-3-2H3zM12 6v15',
  '1007': 'M4 20V11h4v9m4 0V7h4v13m4 0V3M2 21h20',
  '1008': 'm14 4 6 6-4 4-2-2-7 8-4-4 8-7-2-2z',
};

export const CategoryIcon = ({
  id,
  className,
}: {
  id: string;
  className?: string;
}) => (
  <svg
    className={className}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={1.7}
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden="true"
  >
    <path
      d={paths[id] || 'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z'}
    />
  </svg>
);
