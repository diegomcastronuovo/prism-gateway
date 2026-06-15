'use client'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export type ObservableFieldForm = {
  path: string
  type: 'text' | 'json' | 'number'
  role: 'input' | 'output'
}

interface ObservableFieldsEditorProps {
  value: ObservableFieldForm[]
  onChange: (value: ObservableFieldForm[]) => void
  disabled?: boolean
}

const TYPE_OPTIONS: ObservableFieldForm['type'][] = ['text', 'json', 'number']
const ROLE_OPTIONS: ObservableFieldForm['role'][] = ['input', 'output']

export function ObservableFieldsEditor({ value, onChange, disabled }: ObservableFieldsEditorProps) {
  const handleUpdate = (index: number, patch: Partial<ObservableFieldForm>) => {
    const next = value.map((field, idx) => (idx === index ? { ...field, ...patch } : field))
    onChange(next)
  }

  const handleRemove = (index: number) => {
    const next = value.filter((_, idx) => idx !== index)
    onChange(next)
  }

  const handleAdd = () => {
    onChange([...value, { path: '', type: 'text', role: 'input' }])
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <Label className="font-medium">Observable Fields</Label>
          <p className="text-xs text-muted-foreground">
            Define which ML input and output fields can be logged and audited.
          </p>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={handleAdd} disabled={disabled}>
          Add field
        </Button>
      </div>

      {value.length === 0 ? (
        <p className="text-sm text-muted-foreground">No observable fields configured</p>
      ) : (
        <div className="space-y-2">
          {value.map((field, index) => (
            <div key={`${index}`} className="grid gap-2 md:grid-cols-[1.5fr_1fr_1fr_auto]">
              <Input
                placeholder="input.features"
                value={field.path}
                onChange={(event) => handleUpdate(index, { path: event.target.value })}
                disabled={disabled}
              />
              <Select
                value={field.type}
                onValueChange={(val) => handleUpdate(index, { type: val as ObservableFieldForm['type'] })}
                disabled={disabled}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Type" />
                </SelectTrigger>
                <SelectContent>
                  {TYPE_OPTIONS.map((option) => (
                    <SelectItem key={option} value={option}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select
                value={field.role}
                onValueChange={(val) => handleUpdate(index, { role: val as ObservableFieldForm['role'] })}
                disabled={disabled}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Role" />
                </SelectTrigger>
                <SelectContent>
                  {ROLE_OPTIONS.map((option) => (
                    <SelectItem key={option} value={option}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => handleRemove(index)}
                disabled={disabled}
              >
                Remove
              </Button>
            </div>
          ))}
        </div>
      )}
      <p className="text-xs text-muted-foreground">Only applies to models of type ML.</p>
    </div>
  )
}
