package main

import (
	"context"
	"fmt"
	"log"

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
	Endpoint  types.String `tfsdk:"endpoint"`
	Endpoints types.List   `tfsdk:"endpoints"`
	Token     types.String `tfsdk:"token"`
	Username  types.String `tfsdk:"username"`
	Password  types.String `tfsdk:"password"`
}

func (p *CephProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ceph"
	resp.Version = p.version
}

func (p *CephProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerSchema.Schema{
		Attributes: map[string]providerSchema.Attribute{
			"endpoint": providerSchema.StringAttribute{
				MarkdownDescription: "The Ceph API endpoint URL",
				Optional:            true,
			},
			"endpoints": providerSchema.ListAttribute{
				ElementType:         types.StringType,
				MarkdownDescription: "The Ceph API endpoint URLs",
				Optional:            true,
			},
			"token": providerSchema.StringAttribute{
				MarkdownDescription: "The token to use for the provider",
				Optional:            true,
				Sensitive:           true,
			},
			"username": providerSchema.StringAttribute{
				MarkdownDescription: "The username for Ceph authentication",
				Optional:            true,
			},
			"password": providerSchema.StringAttribute{
				MarkdownDescription: "The password for Ceph authentication",
				Optional:            true,
				Sensitive:           true,
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
	username := data.Username.ValueString()
	password := data.Password.ValueString()

	// Either token or username/password must be provided
	if token == "" && (username == "" || password == "") {
		resp.Diagnostics.AddError(
			"Missing Configuration",
			"Either token or both username and password must be configured",
		)
		return
	}

	var endpoints []string
	if endpoint != "" {
		endpoints = append(endpoints, endpoint)
	}
	for _, endpoint := range data.Endpoints.Elements() {
		endpoints = append(endpoints, endpoint.(types.String).ValueString())
	}
	if len(endpoints) == 0 {
		resp.Diagnostics.AddError(
			"Missing Configuration",
			"A provider endpoint must be configured",
		)
		return
	}

	// Configure the Ceph API client with authentication
	cephClient := &CephAPIClient{}
	err := cephClient.Configure(ctx, endpoints, username, password, token)
	if err != nil {
		resp.Diagnostics.AddError(
			"Authentication Error",
			fmt.Sprintf("Failed to configure Ceph API client: %s", err),
		)
		return
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
	client *CephAPIClient
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

	client, ok := req.ProviderData.(*CephAPIClient)

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

	entity := data.Entity.ValueString()
	keyringRaw, err := d.client.ClusterExportUser(ctx, entity)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to export user from Ceph API: %s", err),
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
