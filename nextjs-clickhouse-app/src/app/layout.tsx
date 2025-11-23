'use client';

import { ApolloProvider } from '@apollo/client';
import { apolloClient } from '@/lib/apollo-client';
import './globals.css';

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>
        <ApolloProvider client={apolloClient}>
          <div className="min-h-screen bg-gray-50">
            <nav className="bg-white shadow-sm border-b">
              <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
                <div className="flex justify-between h-16">
                  <div className="flex items-center">
                    <a href="/" className="text-xl font-bold text-gray-900">
                      ClickHouse App
                    </a>
                    <div className="ml-10 flex items-baseline space-x-4">
                      <a
                        href="/"
                        className="text-gray-600 hover:text-gray-900 px-3 py-2 rounded-md text-sm font-medium"
                      >
                        Dashboard
                      </a>
                      <a
                        href="/users"
                        className="text-gray-600 hover:text-gray-900 px-3 py-2 rounded-md text-sm font-medium"
                      >
                        Users
                      </a>
                      <a
                        href="/events"
                        className="text-gray-600 hover:text-gray-900 px-3 py-2 rounded-md text-sm font-medium"
                      >
                        Events
                      </a>
                      <a
                        href="/explorer"
                        className="text-gray-600 hover:text-gray-900 px-3 py-2 rounded-md text-sm font-medium"
                      >
                        Explorer
                      </a>
                    </div>
                  </div>
                </div>
              </div>
            </nav>
            <main className="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
              {children}
            </main>
          </div>
        </ApolloProvider>
      </body>
    </html>
  );
}
