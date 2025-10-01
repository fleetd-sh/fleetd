/**
 * Auth Storage abstraction for SSR-safe token management
 */

interface TokenPair {
  accessToken: string;
  refreshToken: string;
}

/**
 * AuthStorage provides a unified interface for auth token storage
 * that works in both server and client environments
 */
class AuthStorage {
  private readonly isServer = typeof window === "undefined";
  private readonly storageKey = {
    accessToken: "auth_token",
    refreshToken: "refresh_token",
    orgId: "org_id",
    userId: "user_id",
  } as const;

  /**
   * Get access token
   */
  getAccessToken(): string | null {
    if (this.isServer) return null;

    try {
      return localStorage.getItem(this.storageKey.accessToken);
    } catch (error) {
      console.warn("Failed to get access token:", error);
      return null;
    }
  }

  /**
   * Get refresh token
   */
  getRefreshToken(): string | null {
    if (this.isServer) return null;

    try {
      return localStorage.getItem(this.storageKey.refreshToken);
    } catch (error) {
      console.warn("Failed to get refresh token:", error);
      return null;
    }
  }

  /**
   * Get organization ID
   */
  getOrgId(): string | null {
    if (this.isServer) return null;

    try {
      return localStorage.getItem(this.storageKey.orgId);
    } catch (error) {
      console.warn("Failed to get org ID:", error);
      return null;
    }
  }

  /**
   * Get user ID
   */
  getUserId(): string | null {
    if (this.isServer) return null;

    try {
      return localStorage.getItem(this.storageKey.userId);
    } catch (error) {
      console.warn("Failed to get user ID:", error);
      return null;
    }
  }

  /**
   * Set tokens
   */
  setTokens(tokens: TokenPair): void {
    if (this.isServer) return;

    try {
      localStorage.setItem(this.storageKey.accessToken, tokens.accessToken);
      localStorage.setItem(this.storageKey.refreshToken, tokens.refreshToken);
    } catch (error) {
      console.error("Failed to set tokens:", error);
    }
  }

  /**
   * Set organization context
   */
  setOrgId(orgId: string): void {
    if (this.isServer) return;

    try {
      localStorage.setItem(this.storageKey.orgId, orgId);
    } catch (error) {
      console.error("Failed to set org ID:", error);
    }
  }

  /**
   * Set user ID
   */
  setUserId(userId: string): void {
    if (this.isServer) return;

    try {
      localStorage.setItem(this.storageKey.userId, userId);
    } catch (error) {
      console.error("Failed to set user ID:", error);
    }
  }

  /**
   * Clear all auth data
   */
  clear(): void {
    if (this.isServer) return;

    try {
      for (const key of Object.values(this.storageKey)) {
        localStorage.removeItem(key);
      }
    } catch (error) {
      console.error("Failed to clear auth storage:", error);
    }
  }

  /**
   * Check if user is authenticated
   */
  isAuthenticated(): boolean {
    return !!this.getAccessToken();
  }

  /**
   * Get auth headers for requests
   */
  getAuthHeaders(): Record<string, string> {
    const headers: Record<string, string> = {};

    const token = this.getAccessToken();
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }

    const orgId = this.getOrgId();
    if (orgId) {
      headers["X-Organization-ID"] = orgId;
    }

    return headers;
  }
}

// Export singleton instance
export const authStorage = new AuthStorage();

// Export type for testing/mocking
export type { AuthStorage };
