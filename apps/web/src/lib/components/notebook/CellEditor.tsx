import { MonacoEditor } from '@/lib/components/MonacoEditor';

interface Props {
  value: string;
  language: string;
  minHeight?: number;
  onChange?: (value: string) => void;
  onBlur?: (value: string) => void;
}

export function CellEditor({ value, language, minHeight = 160, onChange, onBlur }: Props) {
  return <MonacoEditor value={value} language={language} minHeight={minHeight} onChange={onChange} onBlur={onBlur} />;
}
