/**
 * Query Performance Monitor Component
 *
 * Displays real-time GraphQL query performance metrics, slow query history,
 * and optimization hints for database tuning.
 */

'use client';

import React, { useState, useCallback } from 'react';
import {
  useQueryStats,
  useSlowQueryHistory,
  useTopSlowOperations,
  useQueryLoggerConfig,
} from '../lib/logger';
import type { QueryLogEntry } from '../lib/logger';

interface QueryPerformanceMonitorProps {
  /** Initial collapsed state */
  defaultCollapsed?: boolean;
  /** Show configuration panel */
  showConfig?: boolean;
  /** Position on screen */
  position?: 'bottom-right' | 'bottom-left' | 'top-right' | 'top-left';
}

export function QueryPerformanceMonitor({
  defaultCollapsed = true,
  showConfig = true,
  position = 'bottom-right',
}: QueryPerformanceMonitorProps) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);
  const [activeTab, setActiveTab] = useState<'stats' | 'slow' | 'operations' | 'config'>('stats');

  const { stats, refresh, reset } = useQueryStats(3000);
  const { history } = useSlowQueryHistory(3000);
  const topSlowOperations = useTopSlowOperations(5);
  const { config, updateConfig, enable, disable } = useQueryLoggerConfig();

  const positionStyles: Record<string, React.CSSProperties> = {
    'bottom-right': { bottom: 16, right: 16 },
    'bottom-left': { bottom: 16, left: 16 },
    'top-right': { top: 16, right: 16 },
    'top-left': { top: 16, left: 16 },
  };

  const formatDuration = (ms: number) => {
    if (ms < 1) return '<1ms';
    if (ms < 1000) return `${ms.toFixed(0)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  const getSlowLevelColor = (level: QueryLogEntry['slowLevel']) => {
    switch (level) {
      case 'critical': return '#ef4444';
      case 'very_slow': return '#f97316';
      case 'slow': return '#eab308';
      default: return '#22c55e';
    }
  };

  if (!config.enabled && collapsed) {
    return null;
  }

  return (
    <div
      style={{
        position: 'fixed',
        ...positionStyles[position],
        zIndex: 9999,
        fontFamily: 'system-ui, -apple-system, sans-serif',
        fontSize: '12px',
      }}
    >
      {collapsed ? (
        <button
          onClick={() => setCollapsed(false)}
          style={{
            padding: '8px 12px',
            backgroundColor: '#1f2937',
            color: '#fff',
            border: 'none',
            borderRadius: '6px',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
            boxShadow: '0 4px 12px rgba(0,0,0,0.3)',
          }}
        >
          <span>Query Monitor</span>
          {stats && stats.slowQueries + stats.verySlowQueries + stats.criticalQueries > 0 && (
            <span
              style={{
                backgroundColor: '#ef4444',
                color: '#fff',
                padding: '2px 6px',
                borderRadius: '10px',
                fontSize: '10px',
              }}
            >
              {stats.slowQueries + stats.verySlowQueries + stats.criticalQueries}
            </span>
          )}
        </button>
      ) : (
        <div
          style={{
            width: '400px',
            maxHeight: '500px',
            backgroundColor: '#1f2937',
            borderRadius: '8px',
            boxShadow: '0 4px 20px rgba(0,0,0,0.4)',
            overflow: 'hidden',
          }}
        >
          {/* Header */}
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              padding: '12px 16px',
              backgroundColor: '#111827',
              borderBottom: '1px solid #374151',
            }}
          >
            <span style={{ color: '#fff', fontWeight: 600 }}>Query Performance</span>
            <div style={{ display: 'flex', gap: '8px' }}>
              <button
                onClick={refresh}
                style={{
                  padding: '4px 8px',
                  backgroundColor: '#374151',
                  color: '#9ca3af',
                  border: 'none',
                  borderRadius: '4px',
                  cursor: 'pointer',
                  fontSize: '11px',
                }}
              >
                Refresh
              </button>
              <button
                onClick={() => setCollapsed(true)}
                style={{
                  padding: '4px 8px',
                  backgroundColor: 'transparent',
                  color: '#9ca3af',
                  border: 'none',
                  cursor: 'pointer',
                  fontSize: '16px',
                }}
              >
                x
              </button>
            </div>
          </div>

          {/* Tabs */}
          <div
            style={{
              display: 'flex',
              borderBottom: '1px solid #374151',
              backgroundColor: '#111827',
            }}
          >
            {(['stats', 'slow', 'operations', ...(showConfig ? ['config'] : [])] as const).map((tab) => (
              <button
                key={tab}
                onClick={() => setActiveTab(tab as typeof activeTab)}
                style={{
                  flex: 1,
                  padding: '8px',
                  backgroundColor: activeTab === tab ? '#1f2937' : 'transparent',
                  color: activeTab === tab ? '#fff' : '#9ca3af',
                  border: 'none',
                  borderBottom: activeTab === tab ? '2px solid #3b82f6' : '2px solid transparent',
                  cursor: 'pointer',
                  fontSize: '11px',
                  textTransform: 'capitalize',
                }}
              >
                {tab}
                {tab === 'slow' && history.length > 0 && (
                  <span
                    style={{
                      marginLeft: '4px',
                      backgroundColor: '#ef4444',
                      color: '#fff',
                      padding: '1px 4px',
                      borderRadius: '8px',
                      fontSize: '9px',
                    }}
                  >
                    {history.length}
                  </span>
                )}
              </button>
            ))}
          </div>

          {/* Content */}
          <div style={{ padding: '16px', maxHeight: '350px', overflowY: 'auto' }}>
            {activeTab === 'stats' && stats && (
              <div style={{ color: '#d1d5db' }}>
                <div style={{ marginBottom: '16px' }}>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
                    <StatCard label="Total Queries" value={stats.totalQueries} />
                    <StatCard
                      label="Success Rate"
                      value={`${stats.totalQueries > 0 ? ((stats.successfulQueries / stats.totalQueries) * 100).toFixed(1) : 0}%`}
                      color={stats.failedQueries > 0 ? '#ef4444' : '#22c55e'}
                    />
                    <StatCard label="Avg Duration" value={formatDuration(stats.avgDurationMs)} />
                    <StatCard label="Max Duration" value={formatDuration(stats.maxDurationMs)} />
                  </div>
                </div>

                <div style={{ marginBottom: '16px' }}>
                  <h4 style={{ color: '#9ca3af', marginBottom: '8px', fontSize: '11px' }}>Percentiles</h4>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', gap: '8px' }}>
                    <MiniStatCard label="P50" value={formatDuration(stats.p50DurationMs)} />
                    <MiniStatCard label="P90" value={formatDuration(stats.p90DurationMs)} />
                    <MiniStatCard label="P95" value={formatDuration(stats.p95DurationMs)} />
                    <MiniStatCard label="P99" value={formatDuration(stats.p99DurationMs)} />
                  </div>
                </div>

                <div>
                  <h4 style={{ color: '#9ca3af', marginBottom: '8px', fontSize: '11px' }}>Slow Queries</h4>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: '8px' }}>
                    <MiniStatCard label="Slow" value={stats.slowQueries} color="#eab308" />
                    <MiniStatCard label="Very Slow" value={stats.verySlowQueries} color="#f97316" />
                    <MiniStatCard label="Critical" value={stats.criticalQueries} color="#ef4444" />
                  </div>
                </div>

                <button
                  onClick={reset}
                  style={{
                    marginTop: '16px',
                    padding: '6px 12px',
                    backgroundColor: '#374151',
                    color: '#9ca3af',
                    border: 'none',
                    borderRadius: '4px',
                    cursor: 'pointer',
                    fontSize: '11px',
                    width: '100%',
                  }}
                >
                  Reset Statistics
                </button>
              </div>
            )}

            {activeTab === 'slow' && (
              <div style={{ color: '#d1d5db' }}>
                {history.length === 0 ? (
                  <div style={{ textAlign: 'center', color: '#6b7280', padding: '20px' }}>
                    No slow queries detected
                  </div>
                ) : (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                    {history.slice().reverse().slice(0, 10).map((entry) => (
                      <SlowQueryCard key={entry.id} entry={entry} formatDuration={formatDuration} getSlowLevelColor={getSlowLevelColor} />
                    ))}
                  </div>
                )}
              </div>
            )}

            {activeTab === 'operations' && (
              <div style={{ color: '#d1d5db' }}>
                {topSlowOperations.length === 0 ? (
                  <div style={{ textAlign: 'center', color: '#6b7280', padding: '20px' }}>
                    No operations recorded
                  </div>
                ) : (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                    {topSlowOperations.map((op) => (
                      <OperationCard key={op.name} operation={op} formatDuration={formatDuration} />
                    ))}
                  </div>
                )}
              </div>
            )}

            {activeTab === 'config' && showConfig && (
              <div style={{ color: '#d1d5db' }}>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                  <ConfigToggle
                    label="Logging Enabled"
                    checked={config.enabled}
                    onChange={(checked) => (checked ? enable() : disable())}
                  />
                  <ConfigToggle
                    label="Log All Queries"
                    checked={config.logAllQueries}
                    onChange={(checked) => updateConfig({ logAllQueries: checked })}
                  />
                  <ConfigToggle
                    label="Include Variables"
                    checked={config.includeVariables}
                    onChange={(checked) => updateConfig({ includeVariables: checked })}
                  />
                  <ConfigToggle
                    label="Optimization Hints"
                    checked={config.includeOptimizationHints}
                    onChange={(checked) => updateConfig({ includeOptimizationHints: checked })}
                  />

                  <div>
                    <label style={{ display: 'block', marginBottom: '4px', color: '#9ca3af', fontSize: '11px' }}>
                      Slow Threshold (ms)
                    </label>
                    <input
                      type="number"
                      value={config.slowQueryThresholds.slow}
                      onChange={(e) =>
                        updateConfig({
                          slowQueryThresholds: {
                            ...config.slowQueryThresholds,
                            slow: parseInt(e.target.value) || 500,
                          },
                        })
                      }
                      style={{
                        width: '100%',
                        padding: '6px 8px',
                        backgroundColor: '#374151',
                        border: '1px solid #4b5563',
                        borderRadius: '4px',
                        color: '#fff',
                        fontSize: '12px',
                      }}
                    />
                  </div>

                  <div>
                    <label style={{ display: 'block', marginBottom: '4px', color: '#9ca3af', fontSize: '11px' }}>
                      Log Level
                    </label>
                    <select
                      value={config.logLevel}
                      onChange={(e) => updateConfig({ logLevel: e.target.value as typeof config.logLevel })}
                      style={{
                        width: '100%',
                        padding: '6px 8px',
                        backgroundColor: '#374151',
                        border: '1px solid #4b5563',
                        borderRadius: '4px',
                        color: '#fff',
                        fontSize: '12px',
                      }}
                    >
                      <option value="debug">Debug</option>
                      <option value="info">Info</option>
                      <option value="warn">Warn</option>
                      <option value="error">Error</option>
                      <option value="none">None</option>
                    </select>
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function StatCard({ label, value, color }: { label: string; value: string | number; color?: string }) {
  return (
    <div
      style={{
        padding: '12px',
        backgroundColor: '#374151',
        borderRadius: '6px',
      }}
    >
      <div style={{ color: '#9ca3af', fontSize: '10px', marginBottom: '4px' }}>{label}</div>
      <div style={{ color: color || '#fff', fontSize: '18px', fontWeight: 600 }}>{value}</div>
    </div>
  );
}

function MiniStatCard({ label, value, color }: { label: string; value: string | number; color?: string }) {
  return (
    <div
      style={{
        padding: '8px',
        backgroundColor: '#374151',
        borderRadius: '4px',
        textAlign: 'center',
      }}
    >
      <div style={{ color: '#9ca3af', fontSize: '9px' }}>{label}</div>
      <div style={{ color: color || '#fff', fontSize: '12px', fontWeight: 600 }}>{value}</div>
    </div>
  );
}

function SlowQueryCard({
  entry,
  formatDuration,
  getSlowLevelColor,
}: {
  entry: QueryLogEntry;
  formatDuration: (ms: number) => string;
  getSlowLevelColor: (level: QueryLogEntry['slowLevel']) => string;
}) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div
      style={{
        padding: '10px',
        backgroundColor: '#374151',
        borderRadius: '6px',
        borderLeft: `3px solid ${getSlowLevelColor(entry.slowLevel)}`,
      }}
    >
      <div
        style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', cursor: 'pointer' }}
        onClick={() => setExpanded(!expanded)}
      >
        <div>
          <div style={{ fontWeight: 500, color: '#fff', fontSize: '11px' }}>
            {entry.operationName || 'anonymous'}
          </div>
          <div style={{ color: '#9ca3af', fontSize: '10px' }}>
            {entry.timestamp.toLocaleTimeString()}
          </div>
        </div>
        <div style={{ textAlign: 'right' }}>
          <div style={{ color: getSlowLevelColor(entry.slowLevel), fontWeight: 600, fontSize: '12px' }}>
            {formatDuration(entry.durationMs)}
          </div>
          <div style={{ color: '#6b7280', fontSize: '9px', textTransform: 'uppercase' }}>
            {entry.slowLevel.replace('_', ' ')}
          </div>
        </div>
      </div>

      {expanded && entry.optimizationHints && entry.optimizationHints.length > 0 && (
        <div style={{ marginTop: '10px', paddingTop: '10px', borderTop: '1px solid #4b5563' }}>
          <div style={{ color: '#9ca3af', fontSize: '10px', marginBottom: '6px' }}>Optimization Hints:</div>
          {entry.optimizationHints.map((hint, idx) => (
            <div
              key={idx}
              style={{
                color: '#fbbf24',
                fontSize: '10px',
                marginBottom: '4px',
                paddingLeft: '8px',
                borderLeft: '2px solid #fbbf24',
              }}
            >
              {hint}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function OperationCard({
  operation,
  formatDuration,
}: {
  operation: {
    name: string;
    count: number;
    avgDurationMs: number;
    maxDurationMs: number;
    slowCount: number;
    errorCount: number;
    slowRate: number;
    errorRate: number;
  };
  formatDuration: (ms: number) => string;
}) {
  return (
    <div
      style={{
        padding: '10px',
        backgroundColor: '#374151',
        borderRadius: '6px',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '8px' }}>
        <span style={{ fontWeight: 500, color: '#fff', fontSize: '11px' }}>{operation.name}</span>
        <span style={{ color: '#9ca3af', fontSize: '10px' }}>{operation.count} calls</span>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: '8px', fontSize: '10px' }}>
        <div>
          <div style={{ color: '#9ca3af' }}>Avg</div>
          <div style={{ color: '#fff' }}>{formatDuration(operation.avgDurationMs)}</div>
        </div>
        <div>
          <div style={{ color: '#9ca3af' }}>Max</div>
          <div style={{ color: '#fff' }}>{formatDuration(operation.maxDurationMs)}</div>
        </div>
        <div>
          <div style={{ color: '#9ca3af' }}>Slow Rate</div>
          <div style={{ color: operation.slowRate > 10 ? '#ef4444' : '#22c55e' }}>
            {operation.slowRate.toFixed(1)}%
          </div>
        </div>
      </div>
    </div>
  );
}

function ConfigToggle({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
      <span style={{ color: '#d1d5db', fontSize: '12px' }}>{label}</span>
      <button
        onClick={() => onChange(!checked)}
        style={{
          width: '40px',
          height: '20px',
          borderRadius: '10px',
          backgroundColor: checked ? '#3b82f6' : '#4b5563',
          border: 'none',
          cursor: 'pointer',
          position: 'relative',
        }}
      >
        <span
          style={{
            position: 'absolute',
            top: '2px',
            left: checked ? '22px' : '2px',
            width: '16px',
            height: '16px',
            borderRadius: '50%',
            backgroundColor: '#fff',
            transition: 'left 0.2s',
          }}
        />
      </button>
    </div>
  );
}

export default QueryPerformanceMonitor;
