package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerSchema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func providerFunc() provider.Provider {
	return &CephProvider{
		version: version,
	}
}

var (
	_ provider.Provider                       = &CephProvider{}
	_ provider.ProviderWithEphemeralResources = &CephProvider{}
)

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

	var endpointStrings []string
	if endpoint != "" {
		endpointStrings = append(endpointStrings, endpoint)
	}
	for _, endpoint := range data.Endpoints.Elements() {
		endpointStrings = append(endpointStrings, endpoint.(types.String).ValueString())
	}
	if len(endpointStrings) == 0 {
		resp.Diagnostics.AddError(
			"Missing Configuration",
			"A provider endpoint must be configured",
		)
		return
	}

	// Parse and validate all endpoint strings into URL objects
	parsedEndpoints := make([]*url.URL, 0, len(endpointStrings))
	for _, endpointStr := range endpointStrings {
		if endpointStr == "" {
			resp.Diagnostics.AddError(
				"Invalid Configuration",
				"Endpoint cannot be empty",
			)
			return
		}
		if strings.HasSuffix(endpointStr, "/api") {
			resp.Diagnostics.AddError(
				"Invalid Configuration",
				fmt.Sprintf("Endpoint SHOULD NOT end with '/api', got: %s", endpointStr),
			)
			return
		}

		parsedURL, err := url.Parse(endpointStr)
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid Configuration",
				fmt.Sprintf("Unable to parse endpoint URL %s: %s", endpointStr, err),
			)
			return
		}
		parsedEndpoints = append(parsedEndpoints, parsedURL)
	}

	// Configure the Ceph API client with authentication
	cephClient := &CephAPIClient{}
	err := cephClient.Configure(ctx, parsedEndpoints, username, password, token)
	if err != nil {
		resp.Diagnostics.AddError(
			"Authentication Error",
			fmt.Sprintf("Failed to configure Ceph API client: %s", err),
		)
		return
	}

	resp.DataSourceData = cephClient
	resp.ResourceData = cephClient
	resp.EphemeralResourceData = cephClient
}

func (p *CephProvider) EphemeralResources(ctx context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{
		newAuthEphemeralResource,
	}
}

func (p *CephProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newAuthResource,
		newConfigResource,
		newMgrModuleConfigResource,
		newRGWUserResource,
		newRGWS3KeyResource,
	}
}

func (p *CephProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newAuthDataSource,
		newConfigDataSource,
		newConfigValueDataSource,
		newMgrModuleConfigDataSource,
		newRGWUserDataSource,
		newRGWSubuserDataSource,
		newRGWS3KeyDataSource,
		newRGWSwiftKeyDataSource,
	}
}
