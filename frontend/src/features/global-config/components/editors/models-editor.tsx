'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Plus, Trash2 } from 'lucide-react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

interface ModelMock {
  Enabled: boolean
  DelayMinMs: number
  DelayMaxMs: number
  ErrorRate: number
  ErrorStatus: number
  ErrorMessage: string
  FixedResponse: string
}

/** Must match backend / global config JSON: snake_case inside Pricing (see bug_fe_models_pricing_not_persisting) */
interface ModelPricing {
  prompt_per_1m: number
  completion_per_1m: number
}

interface Model {
  Name: string
  Provider: string
  Type: string
  Pricing: ModelPricing
  Mock: ModelMock
}

interface ModelsEditorProps {
  models: Model[]
  onSave: (updatedModels: Model[]) => void
  onCancel: () => void
}

export function ModelsEditor({ models, onSave, onCancel }: ModelsEditorProps) {
  const [editedModels, setEditedModels] = useState<Model[]>(
    JSON.parse(JSON.stringify(models))
  )
  const [expandedRow, setExpandedRow] = useState<number | null>(null)

  const handleModelChange = (index: number, field: keyof Model, value: string | number) => {
    setEditedModels((prev) => {
      const updated = [...prev]
      if (field === 'Pricing' || field === 'Mock') return prev
      updated[index] = { ...updated[index], [field]: value }
      return updated
    })
  }

  const handlePricingChange = (index: number, field: keyof ModelPricing, value: number) => {
    setEditedModels((prev) => {
      const updated = [...prev]
      updated[index] = {
        ...updated[index],
        Pricing: { ...updated[index].Pricing, [field]: value },
      }
      return updated
    })
  }

  const handleMockChange = (index: number, field: keyof ModelMock, value: boolean | number | string) => {
    setEditedModels((prev) => {
      const updated = [...prev]
      updated[index] = {
        ...updated[index],
        Mock: { ...updated[index].Mock, [field]: value },
      }
      return updated
    })
  }

  const handleAddModel = () => {
    const newModel: Model = {
      Name: '',
      Provider: '',
      Type: '',
      Pricing: { prompt_per_1m: 0, completion_per_1m: 0 },
      Mock: {
        Enabled: false,
        DelayMinMs: 0,
        DelayMaxMs: 0,
        ErrorRate: 0,
        ErrorStatus: 500,
        ErrorMessage: '',
        FixedResponse: '',
      },
    }
    setEditedModels((prev) => [...prev, newModel])
  }

  const handleDeleteModel = (index: number) => {
    setEditedModels((prev) => prev.filter((_, i) => i !== index))
  }

  const handleSave = () => {
    onSave(editedModels)
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h3 className="text-sm font-medium">Edit Models</h3>
        <Button size="sm" variant="outline" onClick={handleAddModel}>
          <Plus className="mr-2 h-4 w-4" />
          Add Model
        </Button>
      </div>

      <div className="border rounded-md">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Provider</TableHead>
              <TableHead>Type</TableHead>
              <TableHead className="text-right">Prompt / 1M</TableHead>
              <TableHead className="text-right">Completion / 1M</TableHead>
              <TableHead>Mock</TableHead>
              <TableHead className="w-[100px]">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {editedModels.map((model, idx) => (
              <>
                <TableRow key={idx}>
                  <TableCell>
                    <Input
                      value={model.Name}
                      onChange={(e) => handleModelChange(idx, 'Name', e.target.value)}
                      className="h-8 text-sm"
                      placeholder="model-name"
                    />
                  </TableCell>
                  <TableCell>
                    <Input
                      value={model.Provider}
                      onChange={(e) => handleModelChange(idx, 'Provider', e.target.value)}
                      className="h-8 text-sm"
                      placeholder="provider"
                    />
                  </TableCell>
                  <TableCell>
                    <Input
                      value={model.Type}
                      onChange={(e) => handleModelChange(idx, 'Type', e.target.value)}
                      className="h-8 text-sm"
                      placeholder="type"
                    />
                  </TableCell>
                  <TableCell>
                    <Input
                      type="number"
                      step="0.01"
                      value={model.Pricing.prompt_per_1m}
                      onChange={(e) =>
                        handlePricingChange(idx, 'prompt_per_1m', parseFloat(e.target.value) || 0)
                      }
                      className="h-8 text-sm text-right"
                    />
                  </TableCell>
                  <TableCell>
                    <Input
                      type="number"
                      step="0.01"
                      value={model.Pricing.completion_per_1m}
                      onChange={(e) =>
                        handlePricingChange(idx, 'completion_per_1m', parseFloat(e.target.value) || 0)
                      }
                      className="h-8 text-sm text-right"
                    />
                  </TableCell>
                  <TableCell>
                    <Button
                      size="sm"
                      variant={expandedRow === idx ? 'default' : 'outline'}
                      onClick={() => setExpandedRow(expandedRow === idx ? null : idx)}
                    >
                      {model.Mock.Enabled ? 'Enabled' : 'Disabled'}
                    </Button>
                  </TableCell>
                  <TableCell>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => handleDeleteModel(idx)}
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </TableCell>
                </TableRow>
                {expandedRow === idx && (
                  <TableRow>
                    <TableCell colSpan={7} className="bg-muted/50">
                      <div className="p-4 space-y-3">
                        <h4 className="text-sm font-semibold mb-3">Mock Configuration</h4>
                        <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
                          <div className="space-y-1">
                            <Label className="text-xs">Enabled</Label>
                            <select
                              value={model.Mock.Enabled ? 'true' : 'false'}
                              onChange={(e) =>
                                handleMockChange(idx, 'Enabled', e.target.value === 'true')
                              }
                              className="flex h-8 w-full rounded-md border border-input bg-background px-3 py-1 text-sm"
                            >
                              <option value="false">Disabled</option>
                              <option value="true">Enabled</option>
                            </select>
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">Delay Min (ms)</Label>
                            <Input
                              type="number"
                              value={model.Mock.DelayMinMs}
                              onChange={(e) =>
                                handleMockChange(idx, 'DelayMinMs', parseInt(e.target.value) || 0)
                              }
                              className="h-8 text-sm"
                            />
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">Delay Max (ms)</Label>
                            <Input
                              type="number"
                              value={model.Mock.DelayMaxMs}
                              onChange={(e) =>
                                handleMockChange(idx, 'DelayMaxMs', parseInt(e.target.value) || 0)
                              }
                              className="h-8 text-sm"
                            />
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">Error Rate (0-1)</Label>
                            <Input
                              type="number"
                              step="0.01"
                              value={model.Mock.ErrorRate}
                              onChange={(e) =>
                                handleMockChange(idx, 'ErrorRate', parseFloat(e.target.value) || 0)
                              }
                              className="h-8 text-sm"
                            />
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">Error Status</Label>
                            <Input
                              type="number"
                              value={model.Mock.ErrorStatus}
                              onChange={(e) =>
                                handleMockChange(idx, 'ErrorStatus', parseInt(e.target.value) || 500)
                              }
                              className="h-8 text-sm"
                            />
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">Error Message</Label>
                            <Input
                              value={model.Mock.ErrorMessage}
                              onChange={(e) => handleMockChange(idx, 'ErrorMessage', e.target.value)}
                              className="h-8 text-sm"
                            />
                          </div>
                          <div className="space-y-1 md:col-span-2 lg:col-span-3">
                            <Label className="text-xs">Fixed Response</Label>
                            <Input
                              value={model.Mock.FixedResponse}
                              onChange={(e) => handleMockChange(idx, 'FixedResponse', e.target.value)}
                              className="h-8 text-sm font-mono"
                            />
                          </div>
                        </div>
                      </div>
                    </TableCell>
                  </TableRow>
                )}
              </>
            ))}
          </TableBody>
        </Table>
      </div>

      <div className="flex gap-2 pt-4 border-t">
        <Button onClick={handleSave}>Save Changes</Button>
        <Button variant="outline" onClick={onCancel}>Cancel</Button>
      </div>
    </div>
  )
}
