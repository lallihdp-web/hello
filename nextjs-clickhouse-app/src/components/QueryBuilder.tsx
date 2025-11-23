'use client';

import { useState } from 'react';

interface Filter {
  field: string;
  operator: string;
  value: string;
}

interface QueryBuilderProps {
  fields: string[];
  onQueryChange: (query: QueryParams) => void;
}

export interface QueryParams {
  selectedFields: string[];
  filters: Filter[];
  orderBy: { field: string; direction: 'asc' | 'desc' } | null;
  limit: number;
  offset: number;
}

const OPERATORS = [
  { value: '_eq', label: 'Equals' },
  { value: '_neq', label: 'Not Equals' },
  { value: '_gt', label: 'Greater Than' },
  { value: '_gte', label: 'Greater Than or Equal' },
  { value: '_lt', label: 'Less Than' },
  { value: '_lte', label: 'Less Than or Equal' },
  { value: '_like', label: 'Like' },
  { value: '_ilike', label: 'Case-insensitive Like' },
  { value: '_in', label: 'In Array' },
];

export function QueryBuilder({ fields, onQueryChange }: QueryBuilderProps) {
  const [selectedFields, setSelectedFields] = useState<string[]>(fields.slice(0, 3));
  const [filters, setFilters] = useState<Filter[]>([]);
  const [orderBy, setOrderBy] = useState<{ field: string; direction: 'asc' | 'desc' } | null>(null);
  const [limit, setLimit] = useState(10);
  const [offset, setOffset] = useState(0);

  const handleFieldToggle = (field: string) => {
    setSelectedFields((prev) =>
      prev.includes(field) ? prev.filter((f) => f !== field) : [...prev, field]
    );
  };

  const addFilter = () => {
    setFilters((prev) => [...prev, { field: fields[0], operator: '_eq', value: '' }]);
  };

  const updateFilter = (index: number, updates: Partial<Filter>) => {
    setFilters((prev) =>
      prev.map((filter, i) => (i === index ? { ...filter, ...updates } : filter))
    );
  };

  const removeFilter = (index: number) => {
    setFilters((prev) => prev.filter((_, i) => i !== index));
  };

  const handleApply = () => {
    onQueryChange({
      selectedFields,
      filters,
      orderBy,
      limit,
      offset,
    });
  };

  return (
    <div className="space-y-6">
      {/* Field Selection */}
      <div>
        <h3 className="text-sm font-medium text-gray-700 mb-2">Select Fields</h3>
        <div className="flex flex-wrap gap-2">
          {fields.map((field) => (
            <label
              key={field}
              className={`inline-flex items-center px-3 py-1 rounded-full text-sm cursor-pointer ${
                selectedFields.includes(field)
                  ? 'bg-blue-100 text-blue-800'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
              }`}
            >
              <input
                type="checkbox"
                checked={selectedFields.includes(field)}
                onChange={() => handleFieldToggle(field)}
                className="sr-only"
              />
              {field}
            </label>
          ))}
        </div>
      </div>

      {/* Filters */}
      <div>
        <div className="flex justify-between items-center mb-2">
          <h3 className="text-sm font-medium text-gray-700">Filters</h3>
          <button onClick={addFilter} className="text-sm text-blue-600 hover:text-blue-800">
            + Add Filter
          </button>
        </div>
        <div className="space-y-2">
          {filters.map((filter, index) => (
            <div key={index} className="flex items-center space-x-2">
              <select
                value={filter.field}
                onChange={(e) => updateFilter(index, { field: e.target.value })}
                className="input flex-1"
              >
                {fields.map((field) => (
                  <option key={field} value={field}>
                    {field}
                  </option>
                ))}
              </select>
              <select
                value={filter.operator}
                onChange={(e) => updateFilter(index, { operator: e.target.value })}
                className="input flex-1"
              >
                {OPERATORS.map((op) => (
                  <option key={op.value} value={op.value}>
                    {op.label}
                  </option>
                ))}
              </select>
              <input
                type="text"
                value={filter.value}
                onChange={(e) => updateFilter(index, { value: e.target.value })}
                placeholder="Value"
                className="input flex-1"
              />
              <button
                onClick={() => removeFilter(index)}
                className="text-red-600 hover:text-red-800"
              >
                Remove
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* Order By */}
      <div>
        <h3 className="text-sm font-medium text-gray-700 mb-2">Order By</h3>
        <div className="flex space-x-2">
          <select
            value={orderBy?.field || ''}
            onChange={(e) =>
              setOrderBy(e.target.value ? { field: e.target.value, direction: 'asc' } : null)
            }
            className="input flex-1"
          >
            <option value="">None</option>
            {fields.map((field) => (
              <option key={field} value={field}>
                {field}
              </option>
            ))}
          </select>
          {orderBy && (
            <select
              value={orderBy.direction}
              onChange={(e) =>
                setOrderBy({ ...orderBy, direction: e.target.value as 'asc' | 'desc' })
              }
              className="input"
            >
              <option value="asc">Ascending</option>
              <option value="desc">Descending</option>
            </select>
          )}
        </div>
      </div>

      {/* Limit & Offset */}
      <div className="flex space-x-4">
        <div className="flex-1">
          <label className="block text-sm font-medium text-gray-700 mb-1">Limit</label>
          <input
            type="number"
            value={limit}
            onChange={(e) => setLimit(parseInt(e.target.value) || 10)}
            min={1}
            max={1000}
            className="input"
          />
        </div>
        <div className="flex-1">
          <label className="block text-sm font-medium text-gray-700 mb-1">Offset</label>
          <input
            type="number"
            value={offset}
            onChange={(e) => setOffset(parseInt(e.target.value) || 0)}
            min={0}
            className="input"
          />
        </div>
      </div>

      {/* Apply Button */}
      <button onClick={handleApply} className="btn btn-primary w-full">
        Apply Query
      </button>
    </div>
  );
}

export default QueryBuilder;
