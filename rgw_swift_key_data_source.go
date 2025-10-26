package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RGWSwiftKeyDataSource{}

func newRGWSwiftKeyDataSource() datasource.DataSource {
	return &RGWSwiftKeyDataSource{}
}

type RGWSwiftKeyDataSource struct {
	client *CephAPIClient
}

type RGWSwiftKeyDataSourceModel struct {
	UserID     types.String `tfsdk:"user_id"`
	SecretKey  types.String `tfsdk:"secret_key"`
	Active     types.Bool   `tfsdk:"active"`
	CreateDate types.String `tfsdk:"create_date"`
}

func (d *RGWSwiftKeyDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_swift_key"
}

func (d *RGWSwiftKeyDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a Ceph RGW Swift key.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"user_id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The subuser ID that owns the Swift key (format: 'user_id:subuser')",
				Required:            true,
			},
			"secret_key": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The Swift secret key",
				Computed:            true,
				Sensitive:           true,
			},
			"active": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the key is active",
				Computed:            true,
			},
			"create_date": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The creation date of the key",
				Computed:            true,
			},
		},
	}
}

func (d *RGWSwiftKeyDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *RGWSwiftKeyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RGWSwiftKeyDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	subuserID := data.UserID.ValueString()

	parts := strings.SplitN(subuserID, ":", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Subuser ID",
			fmt.Sprintf("Swift keys are associated with subusers. The user_id parameter must be in the format 'parent_user:subuser', got: %s", subuserID),
		)
		return
	}
	parentUID := parts[0]

	user, err := d.client.RGWGetUser(ctx, parentUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get RGW user from Ceph API: %s", err),
		)
		return
	}

	var foundKey *CephAPIRGWSwiftKey
	for i := range user.SwiftKeys {
		if user.SwiftKeys[i].User == subuserID {
			foundKey = &user.SwiftKeys[i]
			break
		}
	}

	if foundKey == nil {
		resp.Diagnostics.AddError(
			"Swift Key Not Found",
			fmt.Sprintf("Swift key not found for subuser %s", subuserID),
		)
		return
	}

	data.SecretKey = types.StringValue(foundKey.SecretKey)
	data.Active = types.BoolValue(foundKey.Active)
	if foundKey.CreateDate != "" {
		data.CreateDate = types.StringValue(foundKey.CreateDate)
	} else {
		data.CreateDate = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
