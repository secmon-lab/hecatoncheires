import { Page } from '@playwright/test';

/**
 * Helper functions for GraphQL API calls
 */

export interface GraphQLRequest {
  query: string;
  variables?: Record<string, any>;
}

export interface GraphQLResponse<T = any> {
  data?: T;
  errors?: Array<{ message: string }>;
}

/**
 * Execute a GraphQL query or mutation
 */
export async function executeGraphQL<T = any>(
  page: Page,
  request: GraphQLRequest
): Promise<GraphQLResponse<T>> {
  const baseURL = process.env.BASE_URL || 'http://localhost:8080';
  const response = await page.request.post(`${baseURL}/graphql`, {
    data: request,
    headers: {
      'Content-Type': 'application/json',
    },
  });

  const body = await response.json();
  return body as GraphQLResponse<T>;
}

/**
 * Create a case via GraphQL API
 */
export async function createCaseViaAPI(
  page: Page,
  workspaceId: string,
  caseData: {
    title: string;
    description: string;
    fields?: Array<{ fieldId: string; value: any }>;
  }
): Promise<any> {
  const mutation = `
    mutation CreateCase($input: CaseInput!) {
      createCase(input: $input) {
        id
        title
        description
      }
    }
  `;

  const result = await executeGraphQL(page, {
    query: mutation,
    variables: {
      input: {
        workspaceId,
        ...caseData,
        fields: caseData.fields || [],
      },
    },
  });

  return result.data?.createCase;
}

/**
 * Delete a case via GraphQL API
 */
export async function deleteCaseViaAPI(
  page: Page,
  workspaceId: string,
  caseId: number
): Promise<void> {
  const mutation = `
    mutation DeleteCase($workspaceId: String!, $id: Int!) {
      deleteCase(workspaceId: $workspaceId, id: $id)
    }
  `;

  await executeGraphQL(page, {
    query: mutation,
    variables: { workspaceId, id: caseId },
  });
}

/**
 * Create an action via GraphQL API
 */
export async function createActionViaAPI(
  page: Page,
  workspaceId: string,
  caseId: number,
  actionData: {
    title: string;
    description: string;
  }
): Promise<any> {
  const mutation = `
    mutation CreateAction($input: ActionInput!) {
      createAction(input: $input) {
        id
        title
        description
      }
    }
  `;

  const result = await executeGraphQL(page, {
    query: mutation,
    variables: {
      input: {
        workspaceId,
        caseId,
        ...actionData,
      },
    },
  });

  return result.data?.createAction;
}
