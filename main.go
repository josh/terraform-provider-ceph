package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerSchema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	version string = "dev"
)

func main() {
	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/josh/ceph",
	}

	err := providerserver.Serve(context.Background(), providerFunc, opts)

	if err != nil {
		log.Fatal(err.Error())
	}
}

// Provider

func providerFunc() provider.Provider {
	return &CephProvider{
		version: version,
	}
}

var _ provider.Provider = &CephProvider{}

type CephProvider struct {
	version string
}

type CephProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Token    types.String `tfsdk:"token"`
}

type CephClient struct {
	endpoint string
	token    string
	client   *http.Client
}

// https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cluster-user-export
type CephAPIUserExportRequest struct {
	Entities []string `json:"entities"`
}

func (p *CephProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ceph"
	resp.Version = p.version
}

func (p *CephProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerSchema.Schema{
		Attributes: map[string]providerSchema.Attribute{
			"endpoint": providerSchema.StringAttribute{
				MarkdownDescription: "Example provider attribute",
				Optional:            true,
			},
			"token": providerSchema.StringAttribute{
				MarkdownDescription: "The token to use for the provider",
				Optional:            true,
			},
		},
	}
}

func (p *CephProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data CephProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := data.Endpoint.ValueString()
	token := data.Token.ValueString()

	if endpoint == "" {
		resp.Diagnostics.AddError(
			"Missing Configuration",
			"The provider endpoint must be configured",
		)
		return
	}

	if token == "" {
		resp.Diagnostics.AddError(
			"Missing Configuration",
			"The provider token must be configured",
		)
		return
	}

	cephClient := &CephClient{
		endpoint: endpoint,
		token:    token,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	resp.DataSourceData = cephClient
	resp.ResourceData = cephClient
}

func (p *CephProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}

func (p *CephProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newAuthDataSource,
	}
}

// auth_data_source

var _ datasource.DataSource = &AuthDataSource{}

func newAuthDataSource() datasource.DataSource {
	return &AuthDataSource{}
}

type AuthDataSource struct {
	client *CephClient
}

type AuthDataSourceModel struct {
	Entity  types.String `tfsdk:"entity"`
	Caps    types.Map    `tfsdk:"caps"`
	Id      types.String `tfsdk:"id"`
	Key     types.String `tfsdk:"key"`
	Keyring types.String `tfsdk:"keyring"`
}

func (d *AuthDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth"
}

func (d *AuthDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a ceph client.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"entity": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The entity name (i.e.: client.admin)",
				Required:            true,
			},
			"caps": dataSourceSchema.MapAttribute{
				ElementType:         types.StringType,
				MarkdownDescription: "The caps of the entity",
				Computed:            true,
			},
			"id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The ID of this resource",
				Computed:            true,
			},
			"key": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The cephx key of the entity",
				Computed:            true,
			},
			"keyring": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The complete cephx keyring as JSON",
				Computed:            true,
			},
		},
	}
}

func (d *AuthDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *CephClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *AuthDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AuthDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := d.client.endpoint
	token := d.client.token
	httpClient := d.client.client

	entity := data.Entity.ValueString()
	requestBody := CephAPIUserExportRequest{
		Entities: []string{entity},
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		resp.Diagnostics.AddError(
			"JSON Encoding Error",
			fmt.Sprintf("Unable to encode request payload: %s", err),
		)
		return
	}

	url := endpoint + "/api/cluster/user/export"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		resp.Diagnostics.AddError(
			"Request Creation Error",
			fmt.Sprintf("Unable to create request: %s", err),
		)
		return
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to make request to Ceph API: %s", err),
		)
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		resp.Diagnostics.AddError(
			"API Response Error",
			fmt.Sprintf("Ceph API returned status %d", httpResp.StatusCode),
		)
		return
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		resp.Diagnostics.AddError(
			"Response Reading Error",
			fmt.Sprintf("Unable to read response body: %s", err),
		)
		return
	}

	var keyringRaw string
	err = json.Unmarshal(body, &keyringRaw)
	if err != nil {
		resp.Diagnostics.AddError(
			"JSON Decoding Error",
			fmt.Sprintf("Unable to decode JSON response: %s", err),
		)
		return
	}

	keyringUsers, err := parseCephKeyring(keyringRaw)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to parse keyring data",
			fmt.Sprintf("Unable to parse keyring data: %s", err),
		)
		return
	} else if len(keyringUsers) > 1 {
		resp.Diagnostics.AddWarning(
			"Ceph export return multiple users",
			fmt.Sprintf("Ceph export returned multiple users: %s", keyringRaw),
		)
	}
	keyringUser := keyringUsers[0]

	data.Id = types.StringValue(keyringUser.Entity)
	data.Caps, _ = types.MapValueFrom(ctx, types.StringType, keyringUser.Caps)
	data.Key = types.StringValue(keyringUser.Key)
	data.Keyring = types.StringValue(keyringRaw)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
